package thermal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorReadsHwmonAndFallbackZone(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "class/hwmon/hwmon0")
	zone := filepath.Join(root, "class/thermal/thermal_zone0")
	if err := os.MkdirAll(hwmon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(zone, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(hwmon, "name"), "coretemp\n")
	write(filepath.Join(hwmon, "temp1_label"), "Package\n")
	write(filepath.Join(hwmon, "temp1_input"), "42000\n")
	write(filepath.Join(hwmon, "fan1_input"), "1200\n")
	write(filepath.Join(hwmon, "in1_input"), "1200\n")
	write(filepath.Join(hwmon, "power1_input"), "5000000\n")
	write(filepath.Join(zone, "type"), "acpitz\n")
	write(filepath.Join(zone, "temp"), "30000\n")
	collector := New(root)
	collector.Now = func() time.Time { return time.Unix(1, 0) }
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := metrics[0].Value.(State)
	if len(state.Sensors) != 5 || state.Sensors[0].Unit == "" {
		t.Fatalf("sensors = %#v", state.Sensors)
	}
}

func TestChannelInfoUnits(t *testing.T) {
	tests := map[string]struct {
		kind, unit string
		divisor    float64
	}{
		"temp1_input":  {"temperature", "C", 1000},
		"fan1_input":   {"fan", "RPM", 1},
		"in1_input":    {"voltage", "V", 1000},
		"power1_input": {"power", "W", 1000000},
	}
	for name, expected := range tests {
		_, kind, unit, divisor := channelInfo(name)
		if kind != expected.kind || unit != expected.unit || divisor != expected.divisor {
			t.Errorf("%s: got %s %s %v", name, kind, unit, divisor)
		}
	}
}
