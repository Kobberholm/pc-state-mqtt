package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCLIOverridesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("host_id = \"file-host\"\nsample_interval = \"10s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	options, err := Parse([]string{"--config", path, "--host-id", "cli-host", "--sample-interval", "2s", "--once"}, "default-host", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.HostID != "cli-host" || options.Config.SampleInterval.Duration != 2*time.Second || !options.Once {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseRequiresExplicitConfig(t *testing.T) {
	if _, err := Parse([]string{"--config", filepath.Join(t.TempDir(), "missing.toml")}, "host", io.Discard); err == nil {
		t.Fatal("expected missing explicit config to fail")
	}
}

func TestParseRejectsUnexpectedArguments(t *testing.T) {
	if _, err := Parse([]string{"unexpected"}, "host", io.Discard); err == nil {
		t.Fatal("expected unexpected argument error")
	}
}

func TestParseTUIUsesResolvedConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("host_id = \"tui-host\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := Parse([]string{"--config", path, "--tui"}, "host", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !options.TUI || options.ConfigPath != path {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseRejectsTUIWithOnce(t *testing.T) {
	if _, err := Parse([]string{"--tui", "--once"}, "host", io.Discard); err == nil {
		t.Fatal("expected conflicting mode error")
	}
}
