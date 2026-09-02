package telemetry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "1.0"

type Retention uint8

const (
	Transient Retention = iota
	Retained
)

type Metric struct {
	Path       []string
	Value      any
	Unit       string
	ObservedAt time.Time
	Retention  Retention
}

type Envelope struct {
	Value      any       `json:"value"`
	ObservedAt time.Time `json:"observed_at"`
	Unit       string    `json:"unit,omitempty"`
}

type Diagnostic struct {
	Collector string `json:"collector"`
	Error     string `json:"error"`
}

type Snapshot struct {
	SchemaVersion string              `json:"schema_version"`
	HostID        string              `json:"host_id"`
	ObservedAt    time.Time           `json:"observed_at"`
	Metrics       map[string]Envelope `json:"metrics"`
	Diagnostics   []Diagnostic        `json:"diagnostics,omitempty"`
}

func (metric Metric) Envelope() Envelope {
	return Envelope{
		Value:      metric.Value,
		ObservedAt: metric.ObservedAt.UTC(),
		Unit:       metric.Unit,
	}
}

func (metric Metric) Payload() ([]byte, error) {
	payload, err := json.Marshal(metric.Envelope())
	if err != nil {
		return nil, fmt.Errorf("marshal metric payload: %w", err)
	}
	return payload, nil
}

func (metric Metric) Topic(root, hostID string) (string, error) {
	if len(metric.Path) == 0 {
		return "", fmt.Errorf("metric path is empty")
	}

	segments := make([]string, 0, len(metric.Path)+2)
	segments = append(segments, Segment(root), Segment(hostID))
	for _, part := range metric.Path {
		segments = append(segments, Segment(part))
	}
	return strings.Join(segments, "/"), nil
}

func Segment(value string) string {
	if value == "" {
		return "_"
	}

	var result strings.Builder
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '_', character == '.':
			result.WriteRune(character)
		default:
			fmt.Fprintf(&result, "%%%X", character)
		}
	}
	return result.String()
}

func NewSnapshot(hostID string, observedAt time.Time, metrics []Metric, diagnostics []Diagnostic) Snapshot {
	sort.Slice(metrics, func(left, right int) bool {
		return strings.Join(metrics[left].Path, "\x00") < strings.Join(metrics[right].Path, "\x00")
	})

	values := make(map[string]Envelope, len(metrics))
	for _, metric := range metrics {
		path := make([]string, len(metric.Path))
		for index, part := range metric.Path {
			path[index] = Segment(part)
		}
		values[strings.Join(path, "/")] = metric.Envelope()
	}

	return Snapshot{
		SchemaVersion: SchemaVersion,
		HostID:        hostID,
		ObservedAt:    observedAt.UTC(),
		Metrics:       values,
		Diagnostics:   diagnostics,
	}
}
