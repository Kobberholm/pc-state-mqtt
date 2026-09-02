package memory

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type Collector struct {
	ProcRoot string
	Now      func() time.Time
}

func New(procRoot string) *Collector {
	return &Collector{ProcRoot: procRoot, Now: time.Now}
}

func (collector *Collector) Name() string {
	return "memory"
}

func (collector *Collector) Collect(context.Context) ([]telemetry.Metric, error) {
	values, err := readMeminfo(filepath.Join(collector.ProcRoot, "meminfo"))
	if err != nil {
		return nil, err
	}
	required := []string{"MemTotal", "MemAvailable"}
	for _, name := range required {
		if _, exists := values[name]; !exists {
			return nil, fmt.Errorf("memory statistics are missing %s", name)
		}
	}

	values["MemUsed"] = values["MemTotal"] - min(values["MemTotal"], values["MemAvailable"])
	if total, exists := values["SwapTotal"]; exists {
		values["SwapUsed"] = total - min(total, values["SwapFree"])
	}
	fields := []struct {
		key  string
		path string
	}{
		{"MemTotal", "total_bytes"}, {"MemAvailable", "available_bytes"}, {"MemUsed", "used_bytes"},
		{"Cached", "cached_bytes"}, {"Buffers", "buffers_bytes"},
		{"SwapTotal", "swap_total_bytes"}, {"SwapFree", "swap_free_bytes"}, {"SwapUsed", "swap_used_bytes"},
	}
	observedAt := collector.Now().UTC()
	metrics := make([]telemetry.Metric, 0, len(fields))
	for _, field := range fields {
		value, exists := values[field.key]
		if !exists {
			continue
		}
		retention := telemetry.Transient
		if field.key == "MemTotal" || field.key == "SwapTotal" {
			retention = telemetry.Retained
		}
		metrics = append(metrics, telemetry.Metric{
			Path: []string{"memory", field.path}, Value: value, Unit: "bytes",
			ObservedAt: observedAt, Retention: retention,
		})
	}
	return metrics, nil
}

func readMeminfo(path string) (map[string]uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read memory statistics: %w", err)
	}
	defer file.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %s value %q: %w", name, fields[1], err)
		}
		multiplier := uint64(1)
		if len(fields) > 2 {
			if fields[2] != "kB" {
				continue
			}
			multiplier = 1024
		}
		if value > ^uint64(0)/multiplier {
			return nil, fmt.Errorf("%s value overflows bytes", name)
		}
		values[name] = value * multiplier
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan memory statistics: %w", err)
	}
	return values, nil
}
