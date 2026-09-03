package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/systemstate"
	"pc-state-mqtt/pkg/telemetry"
)

func TestSettingsToggleChangesSelectedCollector(t *testing.T) {
	current := newModel(context.Background(), Options{Config: config.Defaults("host")})
	updated, _ := current.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := updated.(model)
	if result.configuration.Collectors.CPU {
		t.Fatal("CPU collector remained enabled")
	}
	if !strings.Contains(result.View(), "CPU setting changed") {
		t.Fatalf("view = %q", result.View())
	}
}

func TestMonitorUsesCurrentFeatureConfiguration(t *testing.T) {
	called := false
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	current := newModel(context.Background(), Options{
		Config: config.Defaults("host"),
		Now:    func() time.Time { return now },
		Collect: func(_ context.Context, configuration config.Config, featureName string, observedAt time.Time) telemetry.Snapshot {
			called = true
			if configuration.Collectors.CPU {
				t.Fatal("disabled CPU collector was not passed to monitor")
			}
			if featureName != "Memory" {
				t.Fatalf("feature = %q", featureName)
			}
			return telemetry.NewSnapshot(configuration.HostID, observedAt, []telemetry.Metric{{
				Path: []string{"memory", "used_bytes"}, Value: uint64(42), Unit: "bytes", ObservedAt: observedAt,
			}}, nil)
		},
		Connect: nil,
	})
	current.configuration.Collectors.CPU = false
	current.mode = monitorView
	monitor, command := current.startCollections(now, true)
	if command == nil {
		t.Fatal("live watch did not start collection")
	}
	message := command()
	updated, _ := monitor.Update(message)
	monitor = updated.(model)
	view := monitor.View()
	if !called || !strings.Contains(view, "memory/used_bytes") || !strings.Contains(view, "42 bytes") {
		t.Fatalf("monitor view = %q", monitor.View())
	}
	if !strings.Contains(view, "not sent") || !strings.Contains(view, "5s") {
		t.Fatalf("live state missing delivery or countdown: %q", view)
	}
	updated, _ = monitor.Update(refreshMsg{now: now.Add(2 * time.Second)})
	monitor = updated.(model)
	if !strings.Contains(monitor.View(), "3s") {
		t.Fatalf("countdown did not advance: %q", monitor.View())
	}
}

type acknowledgedPublisher struct{}

func (acknowledgedPublisher) PublishMetrics(_ context.Context, metrics []telemetry.Metric) (int, error) {
	return len(metrics), nil
}
func (acknowledgedPublisher) PublishAvailability(context.Context, string) error { return nil }
func (acknowledgedPublisher) Close(context.Context) error                       { return nil }

func TestMonitorRecordsAcknowledgedPublish(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	current := newModel(context.Background(), Options{Config: config.Defaults("host"), Now: func() time.Time { return now }})
	current.publisher = acknowledgedPublisher{}
	metric := telemetry.Metric{Path: []string{"memory", "used_bytes"}, Value: 42, ObservedAt: now}
	updated, command := current.Update(snapshotMsg{feature: "Memory", state: systemstate.State{
		Snapshot: telemetry.NewSnapshot("host", now, []telemetry.Metric{metric}, nil), Metrics: []telemetry.Metric{metric},
	}})
	current = updated.(model)
	if command == nil {
		t.Fatal("gather did not schedule publish")
	}
	updated, _ = current.Update(command())
	current = updated.(model)
	if !current.runtime["Memory"].lastSent.Equal(now) || !strings.Contains(current.View(), "Sent 1 Memory metrics") {
		t.Fatalf("runtime = %#v, view = %q", current.runtime["Memory"], current.View())
	}
}

func TestSettingsViewMarksImplementedFeatures(t *testing.T) {
	current := newModel(context.Background(), Options{Config: config.Defaults("host")})
	view := current.View()
	if !strings.Contains(view, "CPU") || !strings.Contains(view, "available") || !strings.Contains(view, "Thermal") || !strings.Contains(view, "Storage") || !strings.Contains(view, "Network") {
		t.Fatalf("view = %q", view)
	}
}

func TestLiveWatchShowsFutureEventDrivenCadence(t *testing.T) {
	current := newModel(context.Background(), Options{Config: config.Defaults("host")})
	current.mode = monitorView
	view := current.View()
	if !strings.Contains(view, "Display") || !strings.Contains(view, "event-driven") {
		t.Fatalf("view = %q", view)
	}
}

func TestFormatCountdown(t *testing.T) {
	tests := map[time.Duration]string{
		0:                      "due",
		500 * time.Millisecond: "1s",
		5 * time.Second:        "5s",
		65 * time.Second:       "1m05s",
	}
	for duration, expected := range tests {
		if actual := formatCountdown(duration); actual != expected {
			t.Errorf("formatCountdown(%s) = %q, want %q", duration, actual, expected)
		}
	}
}
