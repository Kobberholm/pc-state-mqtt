package watcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	colorReset = "\x1b[0m"
	colorTopic = "\x1b[1;36m"
	colorJSON  = "\x1b[38;5;252m"
)

func WriteMessage(output io.Writer, topic string, payload []byte, color bool) error {
	formatted := payload
	var indented bytes.Buffer
	if json.Valid(payload) {
		if err := json.Indent(&indented, payload, "", "  "); err != nil {
			return fmt.Errorf("format JSON payload: %w", err)
		}
		formatted = indented.Bytes()
	}

	if color {
		_, err := fmt.Fprintf(output, "%s%s%s\n%s%s%s\n", colorTopic, topic, colorReset, colorJSON, formatted, colorReset)
		return err
	}
	_, err := fmt.Fprintf(output, "%s\n%s\n", topic, formatted)
	return err
}
