package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pc-state-mqtt/internal/config"
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
	current := newModel(context.Background(), Options{
		Config: config.Defaults("host"),
		Collect: func(_ context.Context, configuration config.Config, observedAt time.Time) telemetry.Snapshot {
			called = true
			if configuration.Collectors.CPU {
				t.Fatal("disabled CPU collector was not passed to monitor")
			}
			return telemetry.NewSnapshot(configuration.HostID, observedAt, []telemetry.Metric{{
				Path: []string{"memory", "used_bytes"}, Value: uint64(42), Unit: "bytes", ObservedAt: observedAt,
			}}, nil)
		},
	})
	current.configuration.Collectors.CPU = false
	updated, command := current.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	monitor := updated.(model)
	if monitor.mode != monitorView || command == nil {
		t.Fatal("monitor mode did not start collection")
	}
	message := command()
	updated, _ = monitor.Update(message)
	monitor = updated.(model)
	if !called || !strings.Contains(monitor.View(), "memory/used_bytes") || !strings.Contains(monitor.View(), "42 bytes") {
		t.Fatalf("monitor view = %q", monitor.View())
	}
}

func TestSettingsViewMarksPlannedFeatures(t *testing.T) {
	current := newModel(context.Background(), Options{Config: config.Defaults("host")})
	view := current.View()
	if !strings.Contains(view, "CPU") || !strings.Contains(view, "available") || !strings.Contains(view, "Thermal") || !strings.Contains(view, "planned") {
		t.Fatalf("view = %q", view)
	}
}
