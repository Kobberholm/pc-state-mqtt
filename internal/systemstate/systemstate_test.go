package systemstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pc-state-mqtt/internal/config"
)

func TestCollectFeatureCollectsOnlyRequestedFeature(t *testing.T) {
	procRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(procRoot, "meminfo"), []byte("MemTotal: 1000 kB\nMemAvailable: 400 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := config.Defaults("host")
	configuration.ProcRoot = procRoot
	snapshot := CollectFeature(context.Background(), configuration, "memory", time.Unix(1, 0))
	if _, exists := snapshot.Metrics["memory/used_bytes"]; !exists {
		t.Fatalf("metrics = %#v", snapshot.Metrics)
	}
	for path := range snapshot.Metrics {
		if len(path) < len("memory/") || path[:len("memory/")] != "memory/" {
			t.Fatalf("unexpected metric %q", path)
		}
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
