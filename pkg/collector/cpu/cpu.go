package cpu

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type sample struct {
	idle  uint64
	total uint64
}

type Collector struct {
	ProcRoot       string
	SysRoot        string
	SampleDuration time.Duration
	Now            func() time.Time
	Wait           func(context.Context, time.Duration) error
}

func New(procRoot, sysRoot string) *Collector {
	return &Collector{
		ProcRoot: procRoot, SysRoot: sysRoot, SampleDuration: 100 * time.Millisecond,
		Now: time.Now, Wait: wait,
	}
}

func (collector *Collector) Name() string {
	return "cpu"
}

func (collector *Collector) Collect(ctx context.Context) ([]telemetry.Metric, error) {
	before, err := readSamples(filepath.Join(collector.ProcRoot, "stat"))
	if err != nil {
		return nil, err
	}
	if err := collector.Wait(ctx, collector.SampleDuration); err != nil {
		return nil, err
	}
	after, err := readSamples(filepath.Join(collector.ProcRoot, "stat"))
	if err != nil {
		return nil, err
	}

	observedAt := collector.Now().UTC()
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	sort.Strings(names)

	metrics := make([]telemetry.Metric, 0, len(names)*2)
	for _, name := range names {
		previous, exists := before[name]
		if !exists {
			continue
		}
		usage, ok := utilization(previous, after[name])
		if !ok {
			continue
		}
		id := strings.TrimPrefix(name, "cpu")
		if id == "" {
			id = "total"
		}
		metrics = append(metrics, telemetry.Metric{
			Path: []string{"cpu", id, "usage_percent"}, Value: usage,
			Unit: "percent", ObservedAt: observedAt,
		})
		if id != "total" {
			metrics = append(metrics, collector.clockMetrics(id, observedAt)...)
		}
	}
	return metrics, nil
}

func readSamples(path string) (map[string]sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read CPU statistics: %w", err)
	}
	defer file.Close()

	samples := make(map[string]sample)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || (fields[0] != "cpu" && !strings.HasPrefix(fields[0], "cpu")) {
			if len(samples) > 0 {
				break
			}
			continue
		}
		values := make([]uint64, 0, len(fields)-1)
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse %s statistic %q: %w", fields[0], field, err)
			}
			values = append(values, value)
		}
		var total uint64
		for _, value := range values {
			total += value
		}
		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		samples[fields[0]] = sample{idle: idle, total: total}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan CPU statistics: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("CPU statistics contain no CPU rows")
	}
	return samples, nil
}

func utilization(before, after sample) (float64, bool) {
	if after.total <= before.total || after.idle < before.idle {
		return 0, false
	}
	totalDelta := after.total - before.total
	idleDelta := after.idle - before.idle
	if idleDelta > totalDelta {
		return 0, false
	}
	return float64(totalDelta-idleDelta) * 100 / float64(totalDelta), true
}

func (collector *Collector) clockMetrics(id string, observedAt time.Time) []telemetry.Metric {
	directory := filepath.Join(collector.SysRoot, "devices/system/cpu", "cpu"+id, "cpufreq")
	files := []struct {
		name string
		path string
	}{
		{"clock_current_mhz", "scaling_cur_freq"},
		{"clock_min_mhz", "scaling_min_freq"},
		{"clock_max_mhz", "scaling_max_freq"},
	}
	metrics := make([]telemetry.Metric, 0, len(files))
	for _, file := range files {
		contents, err := os.ReadFile(filepath.Join(directory, file.path))
		if err != nil {
			continue
		}
		kilohertz, err := strconv.ParseUint(strings.TrimSpace(string(contents)), 10, 64)
		if err != nil {
			continue
		}
		metrics = append(metrics, telemetry.Metric{
			Path: []string{"cpu", id, file.name}, Value: float64(kilohertz) / 1000,
			Unit: "MHz", ObservedAt: observedAt,
		})
	}
	return metrics
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
