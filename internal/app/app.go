package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"pc-state-mqtt/internal/cli"
	"pc-state-mqtt/internal/version"
	"pc-state-mqtt/pkg/telemetry"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt: determine hostname: %v\n", err)
		return 1
	}

	options, err := cli.Parse(args, hostname, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt: %v\n", err)
		return 2
	}
	if options.Version {
		fmt.Fprintln(stdout, version.Current)
		return 0
	}
	if options.Once {
		if err := writeSnapshot(stdout, options.Config.HostID, time.Now()); err != nil {
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

func writeSnapshot(output io.Writer, hostID string, observedAt time.Time) error {
	metrics := []telemetry.Metric{
		{Path: []string{"meta", "app_version"}, Value: version.Current, ObservedAt: observedAt, Retention: telemetry.Retained},
		{Path: []string{"meta", "schema_version"}, Value: telemetry.SchemaVersion, ObservedAt: observedAt, Retention: telemetry.Retained},
	}
	snapshot := telemetry.NewSnapshot(hostID, observedAt, metrics, nil)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}
