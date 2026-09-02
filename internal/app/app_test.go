package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

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
