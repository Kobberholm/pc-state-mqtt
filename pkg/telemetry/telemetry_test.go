package telemetry

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSegmentIsReversibleByConstruction(t *testing.T) {
	tests := map[string]string{
		"plain":        "plain",
		"DP-1":         "DP-1",
		"root/path":    "root%2Fpath",
		"space name":   "space%20name",
		"percent%name": "percent%25name",
		"":             "_",
	}

	for input, expected := range tests {
		if actual := Segment(input); actual != expected {
			t.Errorf("Segment(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestMetricTopicAndPayload(t *testing.T) {
	observedAt := time.Date(2026, 9, 3, 12, 30, 0, 123, time.FixedZone("test", 3600))
	metric := Metric{
		Path:       []string{"network", "veth/foo", "rx_bytes"},
		Value:      uint64(42),
		Unit:       "bytes",
		ObservedAt: observedAt,
	}

	topic, err := metric.Topic("pc-state", "host/name")
	if err != nil {
		t.Fatal(err)
	}
	if expected := "pc-state/host%2Fname/network/veth%2Ffoo/rx_bytes"; topic != expected {
		t.Fatalf("topic = %q, want %q", topic, expected)
	}

	payload, err := metric.Payload()
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(payload, &actual); err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{
		"value":       float64(42),
		"unit":        "bytes",
		"observed_at": "2026-09-03T11:30:00.000000123Z",
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("payload = %#v, want %#v", actual, expected)
	}
}

func TestMetricTopicRejectsEmptyPath(t *testing.T) {
	if _, err := (Metric{}).Topic("pc-state", "host"); err == nil {
		t.Fatal("expected an empty path error")
	}
}

func TestNewSnapshotNormalizesMetricPaths(t *testing.T) {
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	snapshot := NewSnapshot("workstation", now, []Metric{
		{Path: []string{"storage", "/"}, Value: 1, ObservedAt: now},
		{Path: []string{"cpu", "usage_percent"}, Value: 2, ObservedAt: now},
	}, nil)

	if snapshot.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", snapshot.SchemaVersion, SchemaVersion)
	}
	if _, exists := snapshot.Metrics["storage/%2F"]; !exists {
		t.Fatalf("snapshot metrics = %#v", snapshot.Metrics)
	}
}
