package watcher

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"pc-state-mqtt/internal/version"
)

func TestRunHelpExitsSuccessfully(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--help"}, &bytes.Buffer{}, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-color") {
		t.Fatalf("help does not describe color option: %q", stderr.String())
	}
}

func TestRunVersion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--version"}, &stdout, &bytes.Buffer{}); exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != version.Current {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
