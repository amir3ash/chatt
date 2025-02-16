package apiasync

import (
	"errors"

	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"
)

type asyncMetrics struct {
	Count       *metrics.Metric
	NotFoundNum *metrics.Metric
	WaitTime    *metrics.Metric
	NotRecieved *metrics.Metric
}

// registerMetrics registers the metrics for the kafka module in the metrics registry
func registerMetrics(vu modules.VU) (asyncMetrics, error) {
	var err error
	registry := vu.InitEnv().Registry
	asyncMetrics := asyncMetrics{}

	if asyncMetrics.Count, err = registry.NewMetric(
		"async_mesg_count", metrics.Counter); err != nil {
		return asyncMetrics, errors.Unwrap(err)
	}

	if asyncMetrics.NotFoundNum, err = registry.NewMetric(
		"async_mesg_not_found_count", metrics.Rate); err != nil {
		return asyncMetrics, errors.Unwrap(err)
	}

	if asyncMetrics.NotRecieved, err = registry.NewMetric(
		"async_mesg_not_recieved_count", metrics.Counter); err != nil {
		return asyncMetrics, errors.Unwrap(err)
	}

	if asyncMetrics.WaitTime, err = registry.NewMetric(
		"async_mesg_wait_time", metrics.Trend, metrics.Time); err != nil {
		return asyncMetrics, errors.Unwrap(err)
	}

	return asyncMetrics, nil
}
