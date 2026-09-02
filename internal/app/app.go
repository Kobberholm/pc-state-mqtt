package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"pc-state-mqtt/internal/cli"
	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/version"
	"pc-state-mqtt/pkg/collector"
	"pc-state-mqtt/pkg/collector/cpu"
	"pc-state-mqtt/pkg/collector/memory"
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
	metrics := []telemetry.Metric{
		{Path: []string{"meta", "app_version"}, Value: version.Current, ObservedAt: observedAt, Retention: telemetry.Retained},
		{Path: []string{"meta", "schema_version"}, Value: telemetry.SchemaVersion, ObservedAt: observedAt, Retention: telemetry.Retained},
	}
	collectors := make([]collector.Collector, 0, 2)
	if configuration.Collectors.CPU {
		collectors = append(collectors, cpu.New(configuration.ProcRoot, configuration.SysRoot))
	}
	if configuration.Collectors.Memory {
		collectors = append(collectors, memory.New(configuration.ProcRoot))
	}
	result := collector.CollectAll(ctx, configuration.CollectorTimeout.Duration, collectors...)
	metrics = append(metrics, result.Metrics...)
	snapshot := telemetry.NewSnapshot(configuration.HostID, observedAt, metrics, result.Diagnostics)
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}
