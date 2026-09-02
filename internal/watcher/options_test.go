package watcher

import (
	"io"
	"testing"
)

func TestParseOptionsDefaultsToHostTopicAndColor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	options, err := ParseOptions(nil, "test/host", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if options.Topic != "pc-state/test%2Fhost/#" {
		t.Fatalf("topic = %q", options.Topic)
	}
	if !options.Color {
		t.Fatal("color should be enabled by default")
	}
	if options.ClientID != "pc-state-mqtt-test/host-watch" {
		t.Fatalf("client ID = %q", options.ClientID)
	}
}

func TestParseOptionsSupportsCustomTopicAndDisablingColor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	options, err := ParseOptions([]string{"--topic", "pc-state/+/#", "--color=false"}, "host", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if options.Topic != "pc-state/+/#" || options.Color {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseOptionsUsesOverriddenDefaultTopicParts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	options, err := ParseOptions([]string{"--topic-root", "state", "--host-id", "other"}, "host", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if options.Topic != "state/other/#" {
		t.Fatalf("topic = %q", options.Topic)
	}
}
