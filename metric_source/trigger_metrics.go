package metricSource

import (
	"github.com/moira-alert/moira"
)

type setHelper map[string]bool

func newSetHelperFromTriggerPatternMetrics(metrics TriggerPatternMetrics) setHelper {
	result := make(setHelper, len(metrics))
	for metricName := range metrics {
		result[metricName] = true
	}
	return result
}

func (h setHelper) difference(other setHelper) setHelper {
	result := make(setHelper, len(h))
	for metricName := range h {
		result[metricName] = true
	}
	for metricName := range other {
		if _, ok := result[metricName]; ok {
			delete(result, metricName)
		}
	}
	return result
}

func (h setHelper) intersection(other setHelper) setHelper {
	result := make(setHelper, len(h))
	for metricName := range h {
		if _, ok := other[metricName]; ok {
			result[metricName] = true
		}
	}
	return result
}

func (h setHelper) union(other setHelper) setHelper {
	result := make(setHelper, len(h)+len(other))
	for metricName := range h {
		result[metricName] = true
	}
	for metricName := range other {
		result[metricName] = true
	}
	return result
}

func isOneMetricMap(metrics map[string]MetricData) (bool, string) {
	if len(metrics) == 1 {
		for metricName := range metrics {
			return true, metricName
		}
	}
	return false, ""
}

// TriggerPatternMetrics ...
// TODO(litleleprikon): fill
type TriggerPatternMetrics map[string]MetricData

// newTriggerPatternMetricsWithCapacity ...
// TODO(litleleprikon): fill
func newTriggerPatternMetricsWithCapacity(capacity int) TriggerPatternMetrics {
	return make(TriggerPatternMetrics, capacity)
}

// NewTriggerPatternMetrics ..
// TODO(litleleprikon): fill
func NewTriggerPatternMetrics(source FetchedPatternMetrics) TriggerPatternMetrics {
	result := newTriggerPatternMetricsWithCapacity(len(source))
	for _, m := range source {
		result[m.Name] = m
	}
	return result
}

// Populate is a function that takes the list of metric names that first appeared and
// adds metrics with this names and empty values.
func (m TriggerPatternMetrics) Populate(lastMetrics map[string]bool, from, to int64) TriggerPatternMetrics {
	result := newTriggerPatternMetricsWithCapacity(len(m))

	var firstMetric MetricData

	for _, metric := range m {
		firstMetric = metric
		break
	}

	for metricName := range lastMetrics {
		metric, ok := m[metricName]
		if !ok {
			step := defaultStep
			if len(m) > 0 && firstMetric.StepTime != 0 {
				step = firstMetric.StepTime
			}
			metric = *MakeEmptyMetricData(metricName, step, from, to)
		}
		result[metricName] = metric
	}
	return result
}

func (m TriggerPatternMetrics) diff(fullMetrics setHelper) setHelper {
	result := make(setHelper)
	for metricName := range fullMetrics {
		if _, ok := m[metricName]; !ok {
			result[metricName] = true
		}
	}
	return result
}

// TriggerMetrics ...
// TODO(litleleprikon): fill
type TriggerMetrics map[string]TriggerPatternMetrics

// NewTriggerMetricsWithCapacity is a constructor function that creates TriggerMetrics with initialized empty fields
func NewTriggerMetricsWithCapacity(capacity int) TriggerMetrics {
	return make(TriggerMetrics, capacity)
}

// Populate ...
// TODO(litleleprikon): fill
func (m TriggerMetrics) Populate(lastCheck moira.CheckData, from int64, to int64) TriggerMetrics {

	// lastCheckMetricNames := make(map[string]bool, len(triggerChecker.lastCheck.Metrics))
	// for metricName := range triggerChecker.lastCheck.Metrics {
	// 	lastCheckMetricNames[metricName] = true
	// }

	allMetrics := make(map[string]map[string]bool, len(m))

	for targetName, metricName := range lastCheck.MetricsToTargetRelation {
		allMetrics[targetName] = map[string]bool{metricName: true}
	}

	for metricName, metricState := range lastCheck.Metrics {
		for targetName := range metricState.Values {
			if _, ok := lastCheck.MetricsToTargetRelation[targetName]; ok {
				continue
			}
			if _, ok := allMetrics[targetName]; !ok {
				allMetrics[targetName] = make(map[string]bool)
			}
			allMetrics[targetName][metricName] = true
		}
	}
	for targetName, metrics := range m {
		for metricName := range metrics {
			if _, ok := allMetrics[targetName]; !ok {
				allMetrics[targetName] = make(map[string]bool)
			}
			allMetrics[targetName][metricName] = true
		}
	}

	diff := m.Diff()

	for targetName, metrics := range diff {
		for metricName := range metrics {
			allMetrics[targetName][metricName] = true
		}
	}

	result := NewTriggerMetricsWithCapacity(len(allMetrics))
	for targetName, metrics := range allMetrics {
		patternMetrics, ok := m[targetName]
		if !ok {
			patternMetrics = newTriggerPatternMetricsWithCapacity(len(metrics))
		}
		patternMetrics = patternMetrics.Populate(metrics, from, to)
		result[targetName] = patternMetrics
	}
	return result
}

// FilterAloneMetrics ...
// TODO(litleleprikon): fill
func (m TriggerMetrics) FilterAloneMetrics() (TriggerMetrics, MetricsToCheck) {
	result := NewTriggerMetricsWithCapacity(len(m))
	aloneMetrics := make(MetricsToCheck)

	for targetName, patternMetrics := range m {
		if oneMetricMap, metricName := isOneMetricMap(patternMetrics); oneMetricMap {
			aloneMetrics[targetName] = patternMetrics[metricName]
			continue
		}
		result[targetName] = m[targetName]
	}
	return result, aloneMetrics
}

// Diff ...
// TODO(litleleprikon): fill
func (m TriggerMetrics) Diff() map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	if len(m) == 0 {
		return result
	}

	fullMetrics := make(setHelper)

	for _, patternMetrics := range m {
		if oneMetricTarget, _ := isOneMetricMap(patternMetrics); oneMetricTarget {
			continue
		}
		currentMetrics := newSetHelperFromTriggerPatternMetrics(patternMetrics)
		fullMetrics = fullMetrics.union(currentMetrics)
	}

	for targetName, patternMetrics := range m {
		if oneMetricTarget, _ := isOneMetricMap(patternMetrics); oneMetricTarget {
			continue
		}
		diff := patternMetrics.diff(fullMetrics)
		if len(diff) > 0 {
			result[targetName] = diff
		}
	}
	return result
}

// multiMetricsTarget is a function that finds any first target with
// amount of metrics greater than one and returns set with names of this metrics.
func (m TriggerMetrics) multiMetricsTarget() (string, setHelper) {
	commonMetrics := make(setHelper)
	for targetName, metrics := range m {
		if len(metrics) > 1 {
			for metricName := range metrics {
				commonMetrics[metricName] = true
			}
			return targetName, commonMetrics
		}
	}
	return "", nil
}

// ConvertForCheck is a function that converts TriggerMetrics with structure
// map[TargetName]map[MetricName]MetricData to ConvertedTriggerMetrics
// with structure map[MetricName]map[TargetName]MetricData and fill with
// duplicated metrics targets that have only one metric. Second return value is
// a map with names of targets that had only one metric as key and original metric name as value.
func (m TriggerMetrics) ConvertForCheck() TriggerMetricsToCheck {
	result := make(TriggerMetricsToCheck)
	_, commonMetrics := m.multiMetricsTarget()

	hasAtLeastOneMultiMetricsTarget := commonMetrics != nil

	if !hasAtLeastOneMultiMetricsTarget && len(m) <= 1 {
		return result
	}

	for targetName, targetMetrics := range m {
		oneMetricTarget, oneMetricName := isOneMetricMap(targetMetrics)

		for metricName := range commonMetrics {
			if _, ok := result[metricName]; !ok {
				result[metricName] = make(MetricsToCheck, len(m))
			}

			if oneMetricTarget {
				result[metricName][targetName] = m[targetName][oneMetricName]
				continue
			}

			result[metricName][targetName] = m[targetName][metricName]
		}
	}
	return result
}

// MetricsToCheck ...
// TODO(litleleprikon): fill
type MetricsToCheck map[string]MetricData

// MetricName ...
// TODO(litleleprikon): fill
func (m MetricsToCheck) MetricName() string {
	for _, metric := range m {
		return metric.Name
	}
	return ""
}

// GetRelations ...
// TODO(litleleprikon): fill
func (m MetricsToCheck) GetRelations() map[string]string {
	result := make(map[string]string, len(m))
	for targetName, metric := range m {
		result[targetName] = metric.Name
	}
	return result
}

// Merge ...
// TODO(litleleprikon): fill
func (m MetricsToCheck) Merge(other MetricsToCheck) MetricsToCheck {
	result := make(MetricsToCheck, len(m)+len(other))
	for k, v := range m {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}
	return result
}

// TriggerMetricsToCheck ...
// TODO(litleleprikon): fill
type TriggerMetricsToCheck map[string]MetricsToCheck
