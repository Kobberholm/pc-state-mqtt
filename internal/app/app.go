package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"pc-state-mqtt/internal/cli"
	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/systemstate"
	"pc-state-mqtt/internal/tui"
	"pc-state-mqtt/internal/version"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt: determine hostname: %v\n", err)
		return 1
	}

	options, err := cli.Parse(args, hostname, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt: %v\n", err)
		return 2
	}
	if options.Version {
		fmt.Fprintln(stdout, version.Current)
		return 0
	}
	if options.TUI {
		if err := tui.Run(ctx, tui.Options{
			Config: options.Config, ConfigPath: options.ConfigPath,
			Input: os.Stdin, Output: stdout,
		}); err != nil {
			fmt.Fprintf(stderr, "pc-state-mqtt: TUI: %v\n", err)
			return 1
		}
		return 0
	}
	if options.Once {
		if err := writeSnapshot(ctx, stdout, options.Config, time.Now()); err != nil {
			fmt.Fprintf(stderr, "pc-state-mqtt: %v\n", err)
			return 1
		}
		return 0
	}

	select {
	case <-ctx.Done():
		return 0
	default:
		fmt.Fprintln(stderr, "pc-state-mqtt: MQTT daemon mode is not implemented yet; use --once")
		return 1
	}
}

func writeSnapshot(ctx context.Context, output io.Writer, configuration config.Config, observedAt time.Time) error {
	snapshot := systemstate.Collect(ctx, configuration, observedAt)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}
