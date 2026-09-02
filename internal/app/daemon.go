package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/mqttclient"
	"pc-state-mqtt/internal/systemstate"
	"pc-state-mqtt/pkg/telemetry"
)

type publisher interface {
	Connections() <-chan struct{}
	Errors() <-chan error
	PublishMetrics(context.Context, []telemetry.Metric) (int, error)
	PublishAvailability(context.Context, string) error
	Close(context.Context) error
}

type connectPublisher func(context.Context, config.Config) (publisher, error)

type gatherState func(context.Context, config.Config, time.Time) systemstate.State

func runDaemon(ctx context.Context, stderr io.Writer, configuration config.Config) error {
	return runDaemonWith(ctx, stderr, configuration, time.Now, mqttConnect, systemstate.Gather)
}

func mqttConnect(ctx context.Context, configuration config.Config) (publisher, error) {
	return mqttclient.Connect(ctx, configuration)
}

func runDaemonWith(
	ctx context.Context,
	stderr io.Writer,
	configuration config.Config,
	now func() time.Time,
	connect connectPublisher,
	gather gatherState,
) error {
	if err := ctx.Err(); err != nil {
		return nil
	}
	client, err := connect(ctx, configuration)
	if err != nil {
		return err
	}
	defer func() {
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Close(closeContext); err != nil {
			fmt.Fprintf(stderr, "pc-state-mqtt: publish offline availability: %v\n", err)
		}
	}()

	ticker := time.NewTicker(configuration.SampleInterval.Duration)
	defer ticker.Stop()
	publish := func() {
		observedAt := now().UTC()
		state := gather(ctx, configuration, observedAt)
		if ctx.Err() != nil {
			return
		}
		for _, diagnostic := range state.Snapshot.Diagnostics {
			fmt.Fprintf(stderr, "pc-state-mqtt: collector %s: %s\n", diagnostic.Collector, diagnostic.Error)
		}
		if _, err := client.PublishMetrics(ctx, state.Metrics); err != nil && ctx.Err() == nil {
			fmt.Fprintf(stderr, "pc-state-mqtt: publish telemetry: %v\n", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-client.Connections():
			publish()
		case connectionError := <-client.Errors():
			fmt.Fprintf(stderr, "pc-state-mqtt: MQTT connection lost: %v\n", connectionError)
		case <-ticker.C:
			publish()
		}
	}
}
