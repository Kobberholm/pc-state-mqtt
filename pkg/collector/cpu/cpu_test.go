package cpu

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUtilization(t *testing.T) {
	usage, ok := utilization(sample{idle: 40, total: 100}, sample{idle: 60, total: 200})
	if !ok || usage != 80 {
		t.Fatalf("utilization = %v, %v; want 80, true", usage, ok)
	}
	if _, ok := utilization(sample{idle: 40, total: 100}, sample{idle: 20, total: 200}); ok {
		t.Fatal("counter reset should not produce utilization")
	}
}

func TestReadSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stat")
	contents := "cpu  1 2 3 4 5 6 7 8 9 10\ncpu0 11 12 13 14 15 16 17 18 19 20\nintr 1\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	samples, err := readSamples(path)
	if err != nil {
		t.Fatal(err)
	}
	if samples["cpu"].idle != 9 || samples["cpu0"].total != 155 {
		t.Fatalf("samples = %#v", samples)
	}
}

func TestCollectorCombinesCPUsByDefault(t *testing.T) {
	collector := testCollector(t)
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || len(metrics[0].Path) != 1 || metrics[0].Path[0] != "cpu" {
		t.Fatalf("metrics = %#v", metrics)
	}
	state, ok := metrics[0].Value.(State)
	if !ok || state.Total.UsagePercent != 40 || state.Cores["0"].UsagePercent != 40 {
		t.Fatalf("CPU state = %#v", metrics[0].Value)
	}
	if state.Cores["0"].ClockCurrentMHz == nil || *state.Cores["0"].ClockCurrentMHz != 2400 {
		t.Fatalf("core state = %#v", state.Cores["0"])
	}
}

func TestCollectorCanEmitPerCoreMetrics(t *testing.T) {
	collector := testCollector(t)
	collector.PerCoreOutput = true
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 3 {
		t.Fatalf("got %d metrics, want total usage, CPU usage, and current clock", len(metrics))
	}
}

func testCollector(t *testing.T) *Collector {
	t.Helper()
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	sysRoot := filepath.Join(root, "sys")
	if err := os.MkdirAll(filepath.Join(sysRoot, "devices/system/cpu/cpu0/cpufreq"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(procRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	statPath := filepath.Join(procRoot, "stat")
	if err := os.WriteFile(statPath, []byte("cpu 1 0 0 9 0\ncpu0 1 0 0 9 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysRoot, "devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"), []byte("2400000\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	collector := New(procRoot, sysRoot)
	collector.Now = func() time.Time { return time.Unix(1, 0) }
	collector.Wait = func(context.Context, time.Duration) error {
		return os.WriteFile(statPath, []byte("cpu 5 0 0 15 0\ncpu0 5 0 0 15 0\n"), 0o600)
	}
	return collector
}
