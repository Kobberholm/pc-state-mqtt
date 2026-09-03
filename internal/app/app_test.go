package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/version"
	"pc-state-mqtt/pkg/telemetry"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--version"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != version.Current {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunHelpExitsSuccessfully(t *testing.T) {
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--help"}, &bytes.Buffer{}, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-tui") {
		t.Fatalf("help does not describe TUI option: %q", stderr.String())
	}
}

func TestRunOnceWritesSnapshot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--once", "--host-id", "test-host"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var snapshot telemetry.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.HostID != "test-host" || snapshot.SchemaVersion != telemetry.SchemaVersion {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestRunCancelledDaemonExitsCleanly(t *testing.T) {
	context, cancel := context.WithCancel(context.Background())
	cancel()
	if exitCode := Run(context, nil, &bytes.Buffer{}, &bytes.Buffer{}); exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
}

func TestWriteSnapshotKeepsSuccessfulCollectorData(t *testing.T) {
	procRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(procRoot, "meminfo"), []byte("MemTotal: 1000 kB\nMemAvailable: 400 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := config.Defaults("test-host")
	configuration.ProcRoot = procRoot
	configuration.Collectors.CPU = true
	configuration.Collectors.Memory = true
	configuration.Collectors.Thermal = false
	configuration.Collectors.Storage = false
	configuration.Collectors.Network = false
	var output bytes.Buffer
	if err := writeSnapshot(context.Background(), &output, configuration, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	var snapshot telemetry.Snapshot
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if _, exists := snapshot.Metrics["memory"]; !exists {
		t.Fatalf("metrics = %#v", snapshot.Metrics)
	}
	if len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Collector != "cpu" {
		t.Fatalf("diagnostics = %#v", snapshot.Diagnostics)
	}
}
