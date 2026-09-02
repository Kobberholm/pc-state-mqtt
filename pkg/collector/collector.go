package collector

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type Collector interface {
	Name() string
	Collect(context.Context) ([]telemetry.Metric, error)
}

type Result struct {
	Metrics     []telemetry.Metric
	Diagnostics []telemetry.Diagnostic
}

func CollectAll(ctx context.Context, timeout time.Duration, collectors ...Collector) Result {
	type collected struct {
		name    string
		metrics []telemetry.Metric
		err     error
	}

	results := make(chan collected, len(collectors))
	var workers sync.WaitGroup
	for _, current := range collectors {
		workers.Add(1)
		go func() {
			defer workers.Done()
			collectorContext, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			metrics, err := current.Collect(collectorContext)
			results <- collected{name: current.Name(), metrics: metrics, err: err}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()

	var result Result
	for current := range results {
		if current.err != nil {
			result.Diagnostics = append(result.Diagnostics, telemetry.Diagnostic{Collector: current.name, Error: current.err.Error()})
			continue
		}
		result.Metrics = append(result.Metrics, current.metrics...)
	}
	sort.Slice(result.Metrics, func(left, right int) bool {
		return strings.Join(result.Metrics[left].Path, "\x00") < strings.Join(result.Metrics[right].Path, "\x00")
	})
	sort.Slice(result.Diagnostics, func(left, right int) bool {
		return result.Diagnostics[left].Collector < result.Diagnostics[right].Collector
	})
	return result
}
