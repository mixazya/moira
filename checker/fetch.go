package checker

import (
	metricSource "github.com/moira-alert/moira/metric_source"
)

func (triggerChecker *TriggerChecker) fetchTriggerMetrics() (metricSource.FetchedMetrics, error) {
	triggerMetricsData, metrics, err := triggerChecker.fetch()
	if err != nil {
		return triggerMetricsData, err
	}
	triggerChecker.cleanupMetricsValues(metrics, triggerChecker.until)

	if len(triggerChecker.lastCheck.Metrics) == 0 {
		if triggerMetricsData.HasAnyEmptyPatternMetrics() {
			return nil, ErrTriggerHasNoMetrics{}
		}

		if triggerMetricsData.HasOnlyWildcards() {
			return nil, ErrTriggerHasOnlyWildcards{}
		}
	}

	return triggerMetricsData, nil
}

func (triggerChecker *TriggerChecker) fetch() (metricSource.FetchedMetrics, []string, error) {
	triggerMetricsData := metricSource.NewFetchedMetricsWithCapacity(0)
	metricsArr := make([]string, 0)

	isSimpleTrigger := triggerChecker.trigger.IsSimple()
	for targetIndex, target := range triggerChecker.trigger.Targets {
		targetIndex++ // increasing target index to have target names started from 1 instead of 0
		fetchResult, err := triggerChecker.source.Fetch(target, triggerChecker.from, triggerChecker.until, isSimpleTrigger)
		if err != nil {
			return nil, nil, err
		}
		metricsData := fetchResult.GetMetricsData()

		metricsFetchResult, metricsErr := fetchResult.GetPatternMetrics()
		if len(metricsFetchResult) == 0 {
			return nil, nil, ErrTargetHasNoMetrics{targetIndex: targetIndex}
		}
		if metricsErr == nil {
			metricsArr = append(metricsArr, metricsFetchResult...)
		}

		triggerMetricsData.AddMetrics(targetIndex, metricsData)
	}
	return triggerMetricsData, metricsArr, nil
}

func (triggerChecker *TriggerChecker) cleanupMetricsValues(metrics []string, until int64) {
	if len(metrics) > 0 {
		if err := triggerChecker.database.RemoveMetricsValues(metrics, until-triggerChecker.config.MetricsTTLSeconds); err != nil {
			triggerChecker.logger.Error(err.Error())
		}
	}
}
