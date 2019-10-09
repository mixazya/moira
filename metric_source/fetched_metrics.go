package metricSource

import "fmt"

const defaultStep int64 = 60

// FetchedPatternMetrics represents different metrics within one pattern.
type FetchedPatternMetrics []MetricData

// NewFetchedPatternMetricsWithCapacity is a constructor function for patternMetrics
func NewFetchedPatternMetricsWithCapacity(capacity int) FetchedPatternMetrics {
	return make(FetchedPatternMetrics, 0, capacity)
}

// CleanWildcards is a function that removes all wildcarded metrics and returns new PatternMetrics
func (m FetchedPatternMetrics) CleanWildcards() FetchedPatternMetrics {
	result := NewFetchedPatternMetricsWithCapacity(len(m))
	for _, metric := range m {
		if !metric.Wildcard {
			result = append(result, metric)
		}
	}
	return result
}

// Deduplicate is a function that checks if FetchedPatternMetrics have a two or more metrics with
// the same name and returns new FetchedPatternMetrics without duplicates and slice of duplicated metrics names.
func (m FetchedPatternMetrics) Deduplicate() (FetchedPatternMetrics, []string) {
	deduplicated := NewFetchedPatternMetricsWithCapacity(len(m))
	collectedNames := make(setHelper, len(m))
	var duplicates []string
	for _, metric := range m {
		if collectedNames[metric.Name] {
			duplicates = append(duplicates, metric.Name)
		} else {
			deduplicated = append(deduplicated, metric)
		}
		collectedNames[metric.Name] = true
	}
	return deduplicated, duplicates
}

// HasOnlyWildcards is a function that checks PatternMetrics for only wildcards
func (m FetchedPatternMetrics) HasOnlyWildcards() bool {
	for _, timeSeries := range m {
		if !timeSeries.Wildcard {
			return false
		}
	}
	return true
}

// FetchedMetrics represent collections of metrics associated with target name
// There is a map where keys are target names and values are maps of metrics with metric names as keys.
type FetchedMetrics map[string]FetchedPatternMetrics

// NewFetchedMetricsWithCapacity is a constructor function that creates TriggerMetricsData with initialized empty fields
func NewFetchedMetricsWithCapacity(capacity int) FetchedMetrics {
	return make(FetchedMetrics, capacity)
}

// MetricsAmount is a function that returns amount of all metrics in TriggerMetricsData
func (m FetchedMetrics) MetricsAmount() int {
	result := 0
	for _, v := range m {
		result += len(v)
	}
	return result
}

// AddMetrics is a function to add a bunch of metrics sequences to TriggerMetricsData.
func (m FetchedMetrics) AddMetrics(target int, metrics FetchedPatternMetrics) {
	targetName := fmt.Sprintf("t%d", target)
	m[targetName] = metrics
}

// HasOnlyWildcards is a function that checks given targetTimeSeries for only wildcards
func (m FetchedMetrics) HasOnlyWildcards() bool {
	for _, metric := range m {
		if !metric.HasOnlyWildcards() {
			return false
		}
	}
	return true
}

// HasAnyEmptyPatternMetrics is a function that checks that any of inner PatternMetrics is empty
func (m FetchedMetrics) HasAnyEmptyPatternMetrics() bool {
	for _, patternMetrics := range m {
		if len(patternMetrics) == 0 {
			return true
		}
	}
	return false
}
