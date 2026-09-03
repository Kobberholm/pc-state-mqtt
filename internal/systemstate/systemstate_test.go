package systemstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/pkg/collector/memory"
)

func TestCollectFeatureCollectsOnlyRequestedFeature(t *testing.T) {
	procRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(procRoot, "meminfo"), []byte("MemTotal: 1000 kB\nMemAvailable: 400 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := config.Defaults("host")
	configuration.ProcRoot = procRoot
	snapshot := CollectFeature(context.Background(), configuration, "memory", time.Unix(1, 0))
	envelope, exists := snapshot.Metrics["memory"]
	if !exists || len(snapshot.Metrics) != 1 {
		t.Fatalf("metrics = %#v", snapshot.Metrics)
	}
	state, ok := envelope.Value.(memory.State)
	if !ok || state.UsedBytes != 600*1024 {
		t.Fatalf("memory value = %#v", envelope.Value)
	}
}

func TestCollectFeatureSkipsDisabledFeature(t *testing.T) {
	configuration := config.Defaults("host")
	configuration.Collectors.Memory = false
	snapshot := CollectFeature(context.Background(), configuration, "memory", time.Unix(1, 0))
	if len(snapshot.Metrics) != 0 || len(snapshot.Diagnostics) != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestGatherPreservesMetricRetention(t *testing.T) {
	configuration := config.Defaults("host")
	configuration.Collectors.CPU = false
	configuration.Collectors.Memory = false
	configuration.Collectors.Thermal = false
	configuration.Collectors.Storage = false
	configuration.Collectors.Network = false
	state := Gather(context.Background(), configuration, time.Unix(1, 0))
	if len(state.Metrics) != 2 {
		t.Fatalf("metrics = %#v", state.Metrics)
	}
	for _, metric := range state.Metrics {
		if metric.Retention != 1 {
			t.Fatalf("metric retention = %d", metric.Retention)
		}
	}
}

func TestCPUCollectorUsesConfiguredOutputMode(t *testing.T) {
	configuration := config.Defaults("host")
	configuration.Collectors.CPUPerCore = true
	if !cpuCollector(configuration).PerCoreOutput {
		t.Fatal("CPU per-core output was not enabled")
	}
}

func TestMemoryCollectorUsesConfiguredOutputMode(t *testing.T) {
	configuration := config.Defaults("host")
	configuration.Collectors.MemoryPerField = true
	if !memoryCollector(configuration).PerFieldOutput {
		t.Fatal("memory per-field output was not enabled")
	}
}

func TestCoreCollectorsAreRegistered(t *testing.T) {
	configuration := config.Defaults("host")
	registered := enabledCollectors(configuration)
	if len(registered) != 5 {
		t.Fatalf("collectors = %#v", registered)
	}
	for _, name := range []string{"cpu", "memory", "thermal", "storage", "network"} {
		found := false
		for _, current := range registered {
			if current.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("collector %q was not registered", name)
		}
	}
}
