package thermal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type Collector struct {
	SysRoot string
	Now     func() time.Time
}

type State struct {
	Sensors []Reading `json:"sensors"`
}

type Reading struct {
	Chip  string  `json:"chip"`
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func New(sysRoot string) *Collector       { return &Collector{SysRoot: sysRoot, Now: time.Now} }
func (collector *Collector) Name() string { return "thermal" }

func (collector *Collector) Collect(context.Context) ([]telemetry.Metric, error) {
	readings, failures := collector.readHwmon()
	zoneReadings, zoneFailures := collector.readZones(readings)
	readings = append(readings, zoneReadings...)
	failures = append(failures, zoneFailures...)
	sort.Slice(readings, func(i, j int) bool {
		if readings[i].Chip != readings[j].Chip {
			return readings[i].Chip < readings[j].Chip
		}
		if readings[i].Type != readings[j].Type {
			return readings[i].Type < readings[j].Type
		}
		return readings[i].Name < readings[j].Name
	})
	state := State{Sensors: readings}
	observedAt := collector.Now().UTC()
	metric := telemetry.Metric{Path: []string{"thermal"}, Value: state, ObservedAt: observedAt}
	if len(failures) > 0 {
		return []telemetry.Metric{metric}, errors.Join(failures...)
	}
	return []telemetry.Metric{metric}, nil
}

func (collector *Collector) readHwmon() ([]Reading, []error) {
	directories, err := filepath.Glob(filepath.Join(collector.SysRoot, "class/hwmon/hwmon*"))
	if err != nil {
		return nil, []error{fmt.Errorf("scan hwmon: %w", err)}
	}
	sort.Strings(directories)
	var readings []Reading
	var failures []error
	for _, directory := range directories {
		chip := chipIdentity(directory)
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}

			failures = append(failures, fmt.Errorf("read hwmon %s: %w", chip, readErr))
			continue
		}
		used := make(map[string]bool)
		for _, entry := range entries {
			base := entry.Name()
			prefix, kind, unit, divisor := channelInfo(base)
			if prefix == "" || !strings.HasSuffix(base, "_input") {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(directory, base))
			if readErr != nil {
				if os.IsNotExist(readErr) {
					continue
				}
				failures = append(failures, fmt.Errorf("read %s: %w", base, readErr))
				continue
			}
			value, parseErr := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
			if parseErr != nil {
				failures = append(failures, fmt.Errorf("parse %s: %w", base, parseErr))
				continue
			}
			label := strings.TrimSpace(readText(filepath.Join(directory, prefix+"_label")))
			if label == "" {
				label = prefix
			}
			if used[label] {
				label = prefix + "_" + label
			}
			used[label] = true
			readings = append(readings, Reading{Chip: chip, Name: label, Type: kind, Value: value / divisor, Unit: unit})
		}
	}
	return readings, failures
}

func chipIdentity(directory string) string {
	if name := strings.TrimSpace(readText(filepath.Join(directory, "name"))); name != "" {
		return name
	}
	if target, err := filepath.EvalSymlinks(filepath.Join(directory, "device")); err == nil {
		return filepath.Clean(target)
	}
	if raw, err := os.ReadFile(filepath.Join(directory, "device/uevent")); err == nil {
		uevent := string(raw)
		for _, line := range strings.Split(uevent, "\n") {
			if strings.HasPrefix(line, "DEVPATH=") || strings.HasPrefix(line, "MODALIAS=") {
				return strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			}
		}
	}
	return filepath.Clean(directory)
}

func (collector *Collector) readZones(hwmon []Reading) ([]Reading, []error) {
	directories, err := filepath.Glob(filepath.Join(collector.SysRoot, "class/thermal/thermal_zone*"))
	if err != nil {
		return nil, []error{fmt.Errorf("scan thermal zones: %w", err)}
	}
	sort.Strings(directories)
	existing := make(map[string]bool)
	for _, reading := range hwmon {
		existing[strings.ToLower(reading.Name)] = true
	}
	var readings []Reading
	var failures []error
	for _, directory := range directories {
		raw, readErr := os.ReadFile(filepath.Join(directory, "temp"))
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			failures = append(failures, fmt.Errorf("read thermal zone %s: %w", filepath.Base(directory), readErr))
			continue
		}
		value, parseErr := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if parseErr != nil {
			failures = append(failures, fmt.Errorf("parse thermal zone %s: %w", filepath.Base(directory), parseErr))
			continue
		}
		name := strings.TrimSpace(readText(filepath.Join(directory, "type")))
		if name == "" {
			name = filepath.Base(directory)
		}
		if existing[strings.ToLower(name)] {
			continue
		}
		existing[strings.ToLower(name)] = true
		readings = append(readings, Reading{Chip: "thermal_zone", Name: name, Type: "temperature", Value: value / 1000, Unit: "C"})
	}
	return readings, failures
}

func channelInfo(name string) (string, string, string, float64) {
	switch {
	case strings.HasPrefix(name, "temp") && strings.HasSuffix(name, "_input"):
		return strings.TrimSuffix(name, "_input"), "temperature", "C", 1000
	case strings.HasPrefix(name, "fan") && strings.HasSuffix(name, "_input"):
		return strings.TrimSuffix(name, "_input"), "fan", "RPM", 1
	case strings.HasPrefix(name, "in") && strings.HasSuffix(name, "_input"):
		return strings.TrimSuffix(name, "_input"), "voltage", "V", 1000
	case strings.HasPrefix(name, "power") && strings.HasSuffix(name, "_input"):
		return strings.TrimSuffix(name, "_input"), "power", "W", 1000000
	default:
		return "", "", "", 0
	}
}

func readText(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}
