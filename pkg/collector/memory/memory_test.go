package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorNormalizesMemoryAndSwap(t *testing.T) {
	root := t.TempDir()
	contents := "MemTotal: 1000 kB\nMemAvailable: 400 kB\nBuffers: 10 kB\nCached: 100 kB\nSwapTotal: 200 kB\nSwapFree: 50 kB\n"
	if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := New(root)
	collector.Now = func() time.Time { return time.Unix(1, 0) }
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

func TestCollectorRequiresCoreFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte("MemTotal: 1000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).Collect(context.Background()); err == nil {
		t.Fatal("expected missing MemAvailable error")
	}
}
