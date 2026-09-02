package collector

import (
	"context"

	"pc-state-mqtt/pkg/telemetry"
)

type Collector interface {
	Name() string
	Collect(context.Context) ([]telemetry.Metric, error)
}
