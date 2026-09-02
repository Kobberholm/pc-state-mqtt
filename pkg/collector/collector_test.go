package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type fakeCollector struct {
	name    string
	metrics []telemetry.Metric
	err     error
}

func (collector fakeCollector) Name() string { return collector.name }

func (collector fakeCollector) Collect(context.Context) ([]telemetry.Metric, error) {
	return collector.metrics, collector.err
}

func TestCollectAllOrdersResultsAndIsolatesErrors(t *testing.T) {
	result := CollectAll(context.Background(), time.Second,
		fakeCollector{name: "failed", err: errors.New("unavailable")},
		fakeCollector{name: "working", metrics: []telemetry.Metric{
			{Path: []string{"memory", "used"}},
			{Path: []string{"cpu", "usage"}},
		}},
	)
	if len(result.Metrics) != 2 || result.Metrics[0].Path[0] != "cpu" {
		t.Fatalf("metrics = %#v", result.Metrics)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Collector != "failed" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}
