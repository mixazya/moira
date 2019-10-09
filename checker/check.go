package checker

import (
	"fmt"

	"github.com/moira-alert/moira"
	"github.com/moira-alert/moira/expression"
	metricSource "github.com/moira-alert/moira/metric_source"
	"github.com/moira-alert/moira/metric_source/local"
	"github.com/moira-alert/moira/metric_source/remote"
)

const (
	secondsInHour int64 = 3600
	checkPointGap int64 = 120
)

// Check handle trigger and last check and write new state of trigger, if state were change then write new NotificationEvent
func (triggerChecker *TriggerChecker) Check() error {
	triggerChecker.logger.Debugf("Checking trigger %s", triggerChecker.triggerID)
	checkData := newCheckData(triggerChecker.lastCheck, triggerChecker.until)
	triggerMetricsData, err := triggerChecker.fetchTriggerMetrics()
	if err != nil {
		return triggerChecker.handleFetchError(checkData, err)
	}

	preparedMetrics, aloneMetrics, err := triggerChecker.prepareMetrics(triggerMetricsData)
	if err != nil {
		var pass bool
		pass, checkData, err = triggerChecker.handlePrepareError(checkData, err)
		if !pass {
			return err
		}
	}

	checkData.MetricsToTargetRelation = aloneMetrics.GetRelations()
	checkData, err = triggerChecker.check(preparedMetrics, aloneMetrics, checkData)
	if err != nil {
		return triggerChecker.handleUndefinedError(checkData, err)
	}

	checkData.State = moira.StateOK
	checkData.LastSuccessfulCheckTimestamp = checkData.Timestamp
	if checkData.LastSuccessfulCheckTimestamp != 0 {
		checkData, err = triggerChecker.compareTriggerStates(checkData)
		if err != nil {
			return err
		}
	}
	checkData.UpdateScore()
	return triggerChecker.database.SetTriggerLastCheck(triggerChecker.triggerID, &checkData, triggerChecker.trigger.IsRemote)
}

func (triggerChecker *TriggerChecker) handlePrepareError(checkData moira.CheckData, err error) (bool, moira.CheckData, error) {
	switch err.(type) {
	case ErrWrongTriggerTargets, ErrTriggerHasSameMetricNames:
		checkData.State = moira.StateERROR
		checkData.Message = err.Error()
		return true, checkData, nil
	case ErrUnexpectedAloneMetric:
		checkData.State = moira.StateEXCEPTION
		checkData.Message = err.Error()
	default:
		return false, checkData, triggerChecker.handleUndefinedError(checkData, err)
	}

	checkData, err = triggerChecker.compareTriggerStates(checkData)
	if err != nil {
		return false, checkData, err
	}
	checkData.UpdateScore()
	return true, checkData, triggerChecker.database.SetTriggerLastCheck(triggerChecker.triggerID, &checkData, triggerChecker.trigger.IsRemote)
}

func (triggerChecker *TriggerChecker) handleFetchError(checkData moira.CheckData, err error) error {
	switch err.(type) {
	case ErrTriggerHasNoMetrics, ErrTriggerHasOnlyWildcards:
		triggerChecker.logger.Debugf("Trigger %s: %s", triggerChecker.triggerID, err.Error())
		triggerState := triggerChecker.ttlState.ToTriggerState()
		checkData.State = triggerState
		checkData.Message = err.Error()
		if triggerChecker.ttl == 0 {
			// Do not alert when user don't wanna receive
			// NODATA state alerts, but change trigger status
			checkData.UpdateScore()
			return triggerChecker.database.SetTriggerLastCheck(triggerChecker.triggerID, &checkData, triggerChecker.trigger.IsRemote)
		}
	case remote.ErrRemoteTriggerResponse:
		timeSinceLastSuccessfulCheck := checkData.Timestamp - checkData.LastSuccessfulCheckTimestamp
		if timeSinceLastSuccessfulCheck >= triggerChecker.ttl {
			checkData.State = moira.StateEXCEPTION
			checkData.Message = fmt.Sprintf("Remote server unavailable. Trigger is not checked for %d seconds", timeSinceLastSuccessfulCheck)
			checkData, err = triggerChecker.compareTriggerStates(checkData)
		}
		triggerChecker.logger.Errorf("Trigger %s: %s", triggerChecker.triggerID, err.Error())
	case local.ErrUnknownFunction, local.ErrEvalExpr:
		checkData.State = moira.StateEXCEPTION
		checkData.Message = err.Error()
		triggerChecker.logger.Warningf("Trigger %s: %s", triggerChecker.triggerID, err.Error())
	default:
		return triggerChecker.handleUndefinedError(checkData, err)
	}
	checkData, err = triggerChecker.compareTriggerStates(checkData)
	if err != nil {
		return err
	}
	checkData.UpdateScore()
	return triggerChecker.database.SetTriggerLastCheck(triggerChecker.triggerID, &checkData, triggerChecker.trigger.IsRemote)
}

func (triggerChecker *TriggerChecker) handleUndefinedError(checkData moira.CheckData, err error) error {
	triggerChecker.metrics.CheckError.Mark(1)
	triggerChecker.logger.Errorf("Trigger %s check failed: %s", triggerChecker.triggerID, err.Error())
	checkData, err = triggerChecker.compareTriggerStates(checkData)
	if err != nil {
		return err
	}
	checkData.UpdateScore()
	return triggerChecker.database.SetTriggerLastCheck(triggerChecker.triggerID, &checkData, triggerChecker.trigger.IsRemote)
}

// Set new last check timestamp that equal to "until" targets fetch interval
// Do not copy message, if will be set if needed
func newCheckData(lastCheck *moira.CheckData, checkTimeStamp int64) moira.CheckData {
	lastMetrics := make(map[string]moira.MetricState, len(lastCheck.Metrics))
	for k, v := range lastCheck.Metrics {
		lastMetrics[k] = v
	}
	metricsToTargetRelation := make(map[string]string, len(lastCheck.MetricsToTargetRelation))
	for k, v := range lastCheck.MetricsToTargetRelation {
		metricsToTargetRelation[k] = v
	}
	newCheckData := *lastCheck
	newCheckData.Metrics = lastMetrics
	newCheckData.Timestamp = checkTimeStamp
	newCheckData.MetricsToTargetRelation = metricsToTargetRelation
	newCheckData.Message = ""
	return newCheckData
}

func newMetricState(oldMetricState moira.MetricState, newState moira.State, newTimestamp int64, newValues map[string]float64) *moira.MetricState {
	newMetricState := oldMetricState

	// This field always changed in every metric check operation
	newMetricState.State = newState
	newMetricState.Timestamp = newTimestamp
	newMetricState.Values = newValues

	// Always set. This fields only changed by user actions
	newMetricState.Maintenance = oldMetricState.Maintenance
	newMetricState.MaintenanceInfo = oldMetricState.MaintenanceInfo

	// Only can be change while understand that metric in maintenance or not in compareMetricStates logic
	newMetricState.Suppressed = oldMetricState.Suppressed

	// This fields always set in compareMetricStates logic
	// TODO: make sure that this logic can be moved here
	newMetricState.EventTimestamp = 0
	newMetricState.SuppressedState = ""
	return &newMetricState
}

func (triggerChecker *TriggerChecker) prepareMetrics(fetchedMetrics metricSource.FetchedMetrics) (metricSource.TriggerMetricsToCheck, metricSource.MetricsToCheck, error) {
	preparedPatternMetrics := metricSource.NewTriggerMetricsWithCapacity(len(fetchedMetrics))
	duplicates := make(map[string][]string)

	for targetName, patternMetrics := range fetchedMetrics {
		prepared, patternDuplicates := triggerChecker.preparePatternMetrics(patternMetrics)
		preparedPatternMetrics[targetName] = prepared
		if len(patternDuplicates) > 0 {
			duplicates[targetName] = patternDuplicates
		}
	}

	// Compatibility with moira version < 2.6.0
	for _, metricState := range triggerChecker.lastCheck.Metrics {
		metricState.ConvertValue()
	}

	populated := preparedPatternMetrics.Populate(*triggerChecker.lastCheck, triggerChecker.from, triggerChecker.until)

	multiMetricTargets, aloneMetrics := populated.FilterAloneMetrics()

	if len(aloneMetrics) != len(triggerChecker.trigger.AloneMetrics) {
		return nil, nil, NewErrUnexpectedAloneMetric(triggerChecker.trigger.AloneMetrics, aloneMetrics.GetRelations())
	}

	for targetName := range aloneMetrics.GetRelations() {
		if !triggerChecker.trigger.AloneMetrics[targetName] {
			return nil, nil, NewErrUnexpectedAloneMetric(triggerChecker.trigger.AloneMetrics, aloneMetrics.GetRelations())
		}
	}

	converted := multiMetricTargets.ConvertForCheck()
	if len(duplicates) > 0 {
		return converted, aloneMetrics, NewErrTriggerHasSameMetricNames(duplicates)
	}
	return converted, aloneMetrics, nil
}

// preparePatternMetrics is a function that takes PatternMetrics and applies following operations on it:
// PatternMetrics ->
// Remove wildcards ->
// Remove duplicated metrics and collect the names of duplicated metrics ->
// Resulted PatternMetrics
func (triggerChecker *TriggerChecker) preparePatternMetrics(fetchedMetrics metricSource.FetchedPatternMetrics) (metricSource.TriggerPatternMetrics, []string) {
	withoutWildcards := fetchedMetrics.CleanWildcards()
	deduplicated, duplicates := withoutWildcards.Deduplicate()

	result := metricSource.NewTriggerPatternMetrics(deduplicated)

	return result, duplicates
}

func (triggerChecker *TriggerChecker) check(metrics metricSource.TriggerMetricsToCheck, aloneMetrics metricSource.MetricsToCheck, checkData moira.CheckData) (moira.CheckData, error) {

	if len(metrics) == 0 { // Case when trigger have only alone metrics
		metricName := aloneMetrics.MetricName()
		metrics[metricName] = make(metricSource.MetricsToCheck)
	}
	for metricName, targets := range metrics {
		triggerChecker.logger.Debugf("[TriggerID:%s] Checking metrics %s", triggerChecker.triggerID, metricName) // TODO(litleleprikon): Add structured logging instead of [Field:Value]
		targets = targets.Merge(aloneMetrics)
		metricState, needToDeleteMetric, err := triggerChecker.checkTargets(metricName, targets)
		if needToDeleteMetric {
			triggerChecker.logger.Infof("[TriggerID:%s] Remove metric: '%s'", triggerChecker.triggerID, metricName)
			delete(checkData.Metrics, metricName) // TODO(litleleprikon): change to RemoveMetric method of CheckData
			err = triggerChecker.database.RemovePatternsMetrics(triggerChecker.trigger.Patterns)
		} else {
			checkData.Metrics[metricName] = metricState
		}
		if err != nil {
			return checkData, err
		}
	}
	return checkData, nil
}

func (triggerChecker *TriggerChecker) checkTargets(metricName string, metrics metricSource.MetricsToCheck) (lastState moira.MetricState, needToDeleteMetric bool, err error) {

	lastState, metricStates, err := triggerChecker.getMetricStepsStates(metricName, metrics)
	if err != nil {
		return lastState, needToDeleteMetric, err
	}
	for _, currentState := range metricStates {
		lastState, err = triggerChecker.compareMetricStates(metricName, currentState, lastState)
		if err != nil {
			return lastState, needToDeleteMetric, err
		}
	}
	needToDeleteMetric, noDataState := triggerChecker.checkForNoData(metricName, lastState)
	if needToDeleteMetric {
		return lastState, needToDeleteMetric, err
	}
	if noDataState != nil {
		lastState, err = triggerChecker.compareMetricStates(metricName, *noDataState, lastState)
	}
	return lastState, needToDeleteMetric, err
}

func (triggerChecker *TriggerChecker) checkForNoData(metricName string, metricLastState moira.MetricState) (bool, *moira.MetricState) {
	if triggerChecker.ttl == 0 {
		return false, nil
	}
	lastCheckTimeStamp := triggerChecker.lastCheck.Timestamp

	if metricLastState.Timestamp+triggerChecker.ttl >= lastCheckTimeStamp {
		return false, nil
	}
	triggerChecker.logger.Debugf("[TriggerID:%s][MetricName:%s] Metric TTL expired for state %v", triggerChecker.triggerID, metricName, metricLastState)
	if triggerChecker.ttlState == moira.TTLStateDEL && metricLastState.EventTimestamp != 0 {
		return true, nil
	}
	return false, newMetricState(
		metricLastState,
		triggerChecker.ttlState.ToMetricState(),
		lastCheckTimeStamp,
		nil,
	)
}

func (triggerChecker *TriggerChecker) getMetricStepsStates(metricName string, metrics metricSource.MetricsToCheck) (last moira.MetricState, current []moira.MetricState, err error) {
	var startTime int64
	var stepTime int64

	for _, metric := range metrics { // Taking values from any metric
		last = triggerChecker.lastCheck.GetOrCreateMetricState(metricName, metric.StartTime-secondsInHour, triggerChecker.trigger.MuteNewMetrics)
		startTime = metric.StartTime
		stepTime = metric.StepTime
		break
	}

	checkPoint := last.GetCheckPoint(checkPointGap)
	triggerChecker.logger.Debugf("[TriggerID:%s][MetricName:%s] Checkpoint: %v", triggerChecker.triggerID, metricName, checkPoint)

	current = make([]moira.MetricState, 0)

	previousState := last
	for valueTimestamp := startTime; valueTimestamp < triggerChecker.until+stepTime; valueTimestamp += stepTime {
		metricNewState, err := triggerChecker.getMetricDataState(metricName, metrics, previousState, valueTimestamp, checkPoint)
		if err != nil {
			return last, current, err
		}
		if metricNewState == nil {
			continue
		}
		previousState = *metricNewState
		current = append(current, *metricNewState)
	}
	return last, current, nil
}

func (triggerChecker *TriggerChecker) getMetricDataState(metricName string, metrics metricSource.MetricsToCheck, lastState moira.MetricState, valueTimestamp, checkPoint int64) (*moira.MetricState, error) {
	if valueTimestamp <= checkPoint {
		return nil, nil
	}
	triggerExpression, values, noEmptyValues := getExpressionValues(metrics, valueTimestamp)
	if !noEmptyValues {
		return nil, nil
	}
	triggerChecker.logger.Debugf("[TriggerID:%s][MetricName:%s] Values for ts %v: MainTargetValue: %v, additionalTargetValues: %v", triggerChecker.triggerID, metricName, valueTimestamp, triggerExpression.MainTargetValue, triggerExpression.AdditionalTargetsValues)

	triggerExpression.WarnValue = triggerChecker.trigger.WarnValue
	triggerExpression.ErrorValue = triggerChecker.trigger.ErrorValue
	triggerExpression.TriggerType = triggerChecker.trigger.TriggerType
	triggerExpression.PreviousState = lastState.State
	triggerExpression.Expression = triggerChecker.trigger.Expression

	expressionState, err := triggerExpression.Evaluate()
	if err != nil {
		return nil, err
	}

	return newMetricState(
		lastState,
		expressionState,
		valueTimestamp,
		values,
	), nil
}

func getExpressionValues(metrics metricSource.MetricsToCheck, valueTimestamp int64) (*expression.TriggerExpression, map[string]float64, bool) {
	expression := &expression.TriggerExpression{
		AdditionalTargetsValues: make(map[string]float64, len(metrics)-1),
	}
	values := make(map[string]float64, len(metrics))

	firstTarget := true
	for targetName, metric := range metrics {
		value := metric.GetTimestampValue(valueTimestamp)
		values[targetName] = value
		if !moira.IsValidFloat64(value) {
			return expression, values, false
		}
		if firstTarget {
			expression.MainTargetValue = value
			firstTarget = false
			continue
		}
	}
	return expression, values, true
}
