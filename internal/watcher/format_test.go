package watcher

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteMessagePrettyPrintsJSONWithoutColor(t *testing.T) {
	var output bytes.Buffer
	if err := WriteMessage(&output, "pc-state/host/cpu/usage", []byte(`{"value":42,"unit":"percent"}`), false); err != nil {
		t.Fatal(err)
	}
	expected := "pc-state/host/cpu/usage\n{\n  \"value\": 42,\n  \"unit\": \"percent\"\n}\n"
	if output.String() != expected {
		t.Fatalf("output = %q, want %q", output.String(), expected)
	}
}

func TestWriteMessagePreservesPlainText(t *testing.T) {
	var output bytes.Buffer
	if err := WriteMessage(&output, "pc-state/host/availability", []byte("online"), false); err != nil {
		t.Fatal(err)
	}
	if output.String() != "pc-state/host/availability\nonline\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteMessageUsesColorByRequest(t *testing.T) {
	var output bytes.Buffer
	if err := WriteMessage(&output, "topic", []byte(`true`), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("output has no ANSI escapes: %q", output.String())
	}
}
