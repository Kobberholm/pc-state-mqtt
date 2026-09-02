package systemstate

import (
	"context"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/version"
	"pc-state-mqtt/pkg/collector"
	"pc-state-mqtt/pkg/collector/cpu"
	"pc-state-mqtt/pkg/collector/memory"
	"pc-state-mqtt/pkg/telemetry"
)

func Collect(ctx context.Context, configuration config.Config, observedAt time.Time) telemetry.Snapshot {
	metrics := []telemetry.Metric{
		{Path: []string{"meta", "app_version"}, Value: version.Current, ObservedAt: observedAt, Retention: telemetry.Retained},
		{Path: []string{"meta", "schema_version"}, Value: telemetry.SchemaVersion, ObservedAt: observedAt, Retention: telemetry.Retained},
	}
	collectors := enabledCollectors(configuration)
	result := collector.CollectAll(ctx, configuration.CollectorTimeout.Duration, collectors...)
	metrics = append(metrics, result.Metrics...)
	return telemetry.NewSnapshot(configuration.HostID, observedAt, metrics, result.Diagnostics)
}

func enabledCollectors(configuration config.Config) []collector.Collector {
	collectors := make([]collector.Collector, 0, 2)
	if configuration.Collectors.CPU {
		collectors = append(collectors, cpu.New(configuration.ProcRoot, configuration.SysRoot))
	}
	if configuration.Collectors.Memory {
		collectors = append(collectors, memory.New(configuration.ProcRoot))
	}
	return collectors
}
