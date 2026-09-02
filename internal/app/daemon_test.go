package app

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/systemstate"
	"pc-state-mqtt/pkg/telemetry"
)

type fakePublisher struct {
	connections chan struct{}
	errors      chan error
	mu          sync.Mutex
	statuses    []string
	metrics     [][]telemetry.Metric
	closed      bool
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{connections: make(chan struct{}, 2), errors: make(chan error, 1)}
}

func (publisher *fakePublisher) Connections() <-chan struct{} { return publisher.connections }
func (publisher *fakePublisher) Errors() <-chan error         { return publisher.errors }
func (publisher *fakePublisher) PublishMetrics(_ context.Context, metrics []telemetry.Metric) (int, error) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.metrics = append(publisher.metrics, metrics)
	return len(metrics), nil
}
func (publisher *fakePublisher) PublishAvailability(_ context.Context, status string) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.statuses = append(publisher.statuses, status)
	return nil
}
func (publisher *fakePublisher) Close(context.Context) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.closed = true
	return nil
}

func TestDaemonPublishesOnConnectAndReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := newFakePublisher()
	client.connections <- struct{}{}
	client.connections <- struct{}{}
	configuration := config.Defaults("host")
	configuration.SampleInterval.Duration = time.Hour
	done := make(chan error, 1)
	go func() {
		done <- runDaemonWith(ctx, &bytes.Buffer{}, configuration, time.Now,
			func(context.Context, config.Config) (publisher, error) { return client, nil },
			func(_ context.Context, _ config.Config, observedAt time.Time) systemstate.State {
				return systemstate.State{Metrics: []telemetry.Metric{{Path: []string{"cpu", "usage"}, Value: 1, ObservedAt: observedAt}}}
			})
	}()

	deadline := time.After(time.Second)
	for {
		client.mu.Lock()
		published := len(client.metrics)
		client.mu.Unlock()
		if published == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("daemon did not publish after both connections")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.statuses) != 0 {
		t.Fatalf("statuses = %#v", client.statuses)
	}
	if !client.closed {
		t.Fatal("publisher was not closed")
	}
}

func TestDaemonReturnsConnectionError(t *testing.T) {
	expected := errors.New("connection failed")
	err := runDaemonWith(context.Background(), &bytes.Buffer{}, config.Defaults("host"), time.Now,
		func(context.Context, config.Config) (publisher, error) { return nil, expected }, nil)
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}
