package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorCombinesMemoryByDefault(t *testing.T) {
	collector := testCollector(t)
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || len(metrics[0].Path) != 1 || metrics[0].Path[0] != "memory" {
		t.Fatalf("metrics = %#v", metrics)
	}
	state, ok := metrics[0].Value.(State)
	if !ok || state.UsedBytes != 600*1024 || state.SwapUsedBytes == nil || *state.SwapUsedBytes != 150*1024 {
		t.Fatalf("memory state = %#v", metrics[0].Value)
	}
}

func TestCollectorCanEmitPerFieldMetrics(t *testing.T) {
	collector := testCollector(t)
	collector.PerFieldOutput = true
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]uint64)
	for _, metric := range metrics {
		values[metric.Path[1]] = metric.Value.(uint64)
	}
	if values["used_bytes"] != 600*1024 || values["swap_used_bytes"] != 150*1024 {
		t.Fatalf("values = %#v", values)
	}
}

func testCollector(t *testing.T) *Collector {
	t.Helper()
	root := t.TempDir()
	contents := "MemTotal: 1000 kB\nMemAvailable: 400 kB\nBuffers: 10 kB\nCached: 100 kB\nSwapTotal: 200 kB\nSwapFree: 50 kB\n"
	if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := New(root)
	collector.Now = func() time.Time { return time.Unix(1, 0) }
	return collector
}

func TestCollectorRequiresCoreFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte("MemTotal: 1000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).Collect(context.Background()); err == nil {
		t.Fatal("expected missing MemAvailable error")
	}
}
