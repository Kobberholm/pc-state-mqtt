package systemstate

import (
	"context"
	"strings"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/version"
	"pc-state-mqtt/pkg/collector"
	"pc-state-mqtt/pkg/collector/cpu"
	"pc-state-mqtt/pkg/collector/memory"
	"pc-state-mqtt/pkg/collector/network"
	"pc-state-mqtt/pkg/collector/storage"
	"pc-state-mqtt/pkg/collector/thermal"
	"pc-state-mqtt/pkg/telemetry"
)

type State struct {
	Snapshot telemetry.Snapshot
	Metrics  []telemetry.Metric
}

func Collect(ctx context.Context, configuration config.Config, observedAt time.Time) telemetry.Snapshot {
	return Gather(ctx, configuration, observedAt).Snapshot
}

func Gather(ctx context.Context, configuration config.Config, observedAt time.Time) State {
	metrics := []telemetry.Metric{
		{Path: []string{"meta", "app_version"}, Value: version.Current, ObservedAt: observedAt, Retention: telemetry.Retained},
		{Path: []string{"meta", "schema_version"}, Value: telemetry.SchemaVersion, ObservedAt: observedAt, Retention: telemetry.Retained},
	}
	collectors := enabledCollectors(configuration)
	result := collector.CollectAll(ctx, configuration.CollectorTimeout.Duration, collectors...)
	metrics = append(metrics, result.Metrics...)
	return State{
		Snapshot: telemetry.NewSnapshot(configuration.HostID, observedAt, metrics, result.Diagnostics),
		Metrics:  metrics,
	}
}

func enabledCollectors(configuration config.Config) []collector.Collector {
	collectors := make([]collector.Collector, 0, 5)
	if configuration.Collectors.CPU {
		collectors = append(collectors, cpuCollector(configuration))
	}
	if configuration.Collectors.Memory {
		collectors = append(collectors, memoryCollector(configuration))
	}
	if configuration.Collectors.Thermal {
		collectors = append(collectors, thermalCollector(configuration))
	}
	if configuration.Collectors.Storage {
		collectors = append(collectors, storageCollector(configuration))
	}
	if configuration.Collectors.Network {
		collectors = append(collectors, networkCollector(configuration))
	}
	return collectors
}

func CollectFeature(ctx context.Context, configuration config.Config, name string, observedAt time.Time) telemetry.Snapshot {
	return GatherFeature(ctx, configuration, name, observedAt).Snapshot
}

func GatherFeature(ctx context.Context, configuration config.Config, name string, observedAt time.Time) State {
	var selected collector.Collector
	switch strings.ToLower(name) {
	case "cpu":
		if configuration.Collectors.CPU {
			selected = cpuCollector(configuration)
		}
	case "memory":
		if configuration.Collectors.Memory {
			selected = memoryCollector(configuration)
		}
	case "thermal":
		if configuration.Collectors.Thermal {
			selected = thermalCollector(configuration)
		}
	case "storage":
		if configuration.Collectors.Storage {
			selected = storageCollector(configuration)
		}
	case "network":
		if configuration.Collectors.Network {
			selected = networkCollector(configuration)
		}
	}
	if selected == nil {
		return State{Snapshot: telemetry.NewSnapshot(configuration.HostID, observedAt, nil, nil)}
	}
	result := collector.CollectAll(ctx, configuration.CollectorTimeout.Duration, selected)
	return State{
		Snapshot: telemetry.NewSnapshot(configuration.HostID, observedAt, result.Metrics, result.Diagnostics),
		Metrics:  result.Metrics,
	}
}

func cpuCollector(configuration config.Config) *cpu.Collector {
	collector := cpu.New(configuration.ProcRoot, configuration.SysRoot)
	collector.PerCoreOutput = configuration.Collectors.CPUPerCore
	return collector
}

func memoryCollector(configuration config.Config) *memory.Collector {
	collector := memory.New(configuration.ProcRoot)
	collector.PerFieldOutput = configuration.Collectors.MemoryPerField
	return collector
}

func thermalCollector(configuration config.Config) *thermal.Collector {
	return thermal.New(configuration.SysRoot)
}

func storageCollector(configuration config.Config) *storage.Collector {
	return storage.New(configuration.ProcRoot, configuration.SysRoot)
}

func networkCollector(configuration config.Config) *network.Collector {
	return network.New(configuration.SysRoot)
}
