package cpu

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	PerCoreOutput  bool
	Now            func() time.Time
	Wait           func(context.Context, time.Duration) error
}

type State struct {
	Identity CPUIdentity    `json:"identity"`
	Total    CPU            `json:"total"`
	Cores    map[string]CPU `json:"cores"`
}

type CPUIdentity struct {
	ModelName       string   `json:"model_name,omitempty"`
	VendorID        string   `json:"vendor_id,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
	LogicalCPUCount int      `json:"logical_cpu_count"`
	OnlineCPUCount  int      `json:"online_cpu_count"`
	OnlineCPUs      []string `json:"online_cpus,omitempty"`
}

type CPU struct {
	UsagePercent    float64  `json:"usage_percent"`
	Online          *bool    `json:"online,omitempty"`
	ClockCurrentMHz *float64 `json:"clock_current_mhz,omitempty"`
	ClockMinMHz     *float64 `json:"clock_min_mhz,omitempty"`
	ClockMaxMHz     *float64 `json:"clock_max_mhz,omitempty"`
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
	identity := readIdentity(filepath.Join(collector.ProcRoot, "cpuinfo"))
	online := readOnline(collector.SysRoot, after)
	names := make([]string, 0, len(after))
	for name := range after {
		names = append(names, name)
	}
	sort.Strings(names)

	metrics := make([]telemetry.Metric, 0, len(names)*2)
	state := State{Cores: make(map[string]CPU, max(0, len(names)-1)), Identity: identity}
	for _, name := range names {
		if name != "cpu" {
			state.Identity.LogicalCPUCount++
		}
	}
	state.Identity.OnlineCPUCount = len(sortedKeys(online))
	state.Identity.OnlineCPUs = append([]string(nil), sortedKeys(online)...)
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
		cpuState := CPU{UsagePercent: usage}
		if id == "total" {
			state.Total = cpuState
		} else {
			cpuState = collector.clockState(id, cpuState)
			if isOnline, exists := online[id]; exists {
				cpuState.Online = &isOnline
			}
			state.Cores[id] = cpuState
		}
		if collector.PerCoreOutput {
			metrics = append(metrics, telemetry.Metric{
				Path: []string{"cpu", id, "usage_percent"}, Value: usage,
				Unit: "percent", ObservedAt: observedAt,
			})
			if id != "total" {
				metrics = append(metrics, clockMetrics(id, cpuState, observedAt)...)
			}
		}
	}
	if !collector.PerCoreOutput {
		return []telemetry.Metric{{Path: []string{"cpu"}, Value: state, ObservedAt: observedAt}}, nil
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
		if len(fields) < 5 || !cpuRowName(fields[0]) {
			if len(samples) > 0 {
				break
			}
			continue
		}
		values := make([]uint64, 0, len(fields)-1)
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				values = nil
				break
			}
			values = append(values, value)
		}
		if len(values) < 4 {
			continue
		}
		var total uint64
		overflow := false
		for _, value := range values {
			if ^uint64(0)-total < value {
				overflow = true
				break
			}
			total += value
		}
		if overflow {
			continue
		}

		idle := values[3]
		if len(values) > 4 {
			if ^uint64(0)-idle < values[4] {
				continue
			}
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

func (collector *Collector) clockState(id string, state CPU) CPU {
	directory := filepath.Join(collector.SysRoot, "devices/system/cpu", "cpu"+id, "cpufreq")
	files := []struct {
		path  string
		value **float64
	}{
		{"scaling_cur_freq", &state.ClockCurrentMHz},
		{"scaling_min_freq", &state.ClockMinMHz},
		{"scaling_max_freq", &state.ClockMaxMHz},
	}
	for _, file := range files {
		contents, err := os.ReadFile(filepath.Join(directory, file.path))
		if err != nil {
			continue
		}
		kilohertz, err := strconv.ParseUint(strings.TrimSpace(string(contents)), 10, 64)
		if err != nil {
			continue
		}
		megahertz := float64(kilohertz) / 1000
		*file.value = &megahertz
	}
	return state
}

func clockMetrics(id string, state CPU, observedAt time.Time) []telemetry.Metric {
	values := []struct {
		name  string
		value *float64
	}{
		{"clock_current_mhz", state.ClockCurrentMHz},
		{"clock_min_mhz", state.ClockMinMHz},
		{"clock_max_mhz", state.ClockMaxMHz},
	}
	metrics := make([]telemetry.Metric, 0, len(values))
	for _, value := range values {
		if value.value == nil {
			continue
		}
		metrics = append(metrics, telemetry.Metric{
			Path: []string{"cpu", id, value.name}, Value: *value.value,
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

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if values[key] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		left, _ := strconv.Atoi(keys[i])
		right, _ := strconv.Atoi(keys[j])
		return left < right
	})
	return keys
}

func cpuRowName(name string) bool {
	if name == "cpu" {
		return true
	}
	if !strings.HasPrefix(name, "cpu") || len(name) == 3 {
		return false
	}
	_, err := strconv.Atoi(name[3:])
	return err == nil
}

func readIdentity(path string) CPUIdentity {
	identity := CPUIdentity{Architecture: runtime.GOARCH}
	file, err := os.Open(path)
	if err != nil {
		return identity
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "model name", "Processor":
			if identity.ModelName == "" {
				identity.ModelName = value
			}
		case "vendor_id", "CPU implementer":
			if identity.VendorID == "" {
				identity.VendorID = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return identity
	}
	return identity
}

func readOnline(sysRoot string, samples map[string]sample) map[string]bool {
	online := make(map[string]bool)
	for name := range samples {
		if name != "cpu" {
			online[strings.TrimPrefix(name, "cpu")] = false
		}
	}
	data, err := os.ReadFile(filepath.Join(sysRoot, "devices/system/cpu/online"))
	if err == nil {
		for _, id := range parseCPURange(strings.TrimSpace(string(data))) {
			online[id] = true
		}
		if len(sortedKeys(online)) > 0 {
			return online
		}
	}
	for name := range samples {
		if name == "cpu" {
			continue
		}
		id := strings.TrimPrefix(name, "cpu")
		value := true
		if contents, readErr := os.ReadFile(filepath.Join(sysRoot, "devices/system/cpu", name, "online")); readErr == nil {
			value = strings.TrimSpace(string(contents)) != "0"
		}
		online[id] = value
	}
	return online
}

func parseCPURange(value string) []string {
	set := make(map[int]bool)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		bounds := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil || start < 0 {
			continue
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || end < start {
				continue
			}
		}
		for current := start; current <= end; current++ {
			set[current] = true
		}
	}
	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	result := make([]string, len(ids))
	for index, id := range ids {
		result[index] = strconv.Itoa(id)
	}
	return result
}
