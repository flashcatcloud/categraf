//go:build !no_traces

package traces

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/collector/component"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/config/traces"
)

// Collector simply wrapped the OpenTelemetry Collector, which means you can get a full support
// for recving data from and exporting to popular trace vendors (eg. Jaeger or Zipkin).
// For more details, see the official docs:
//
//	https://opentelemetry.io/docs/collector/getting-started
//	https://github.com/open-telemetry/opentelemetry-collector
type Collector struct {
	srv *service
	cfg *traces.Config
}

// New make a Collector instance
func New(cfg *traces.Config) (*Collector, error) {
	buildInfo := component.BuildInfo{
		Command:     "otelcol-categraf",
		Description: "OpenTelemetry Collector for categraf.",
		Version:     config.Version,
	}

	s, err := newService(&settings{
		Config:            cfg.Parsed,
		Factories:         cfg.Factories,
		BuildInfo:         buildInfo,
		AsyncErrorChannel: make(chan error),
		LoggingOptions:    nil,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create trace service: %v", err)
	}

	return &Collector{
		srv: s,
		cfg: cfg,
	}, nil
}

// Run starts the collector
func (c *Collector) Run(ctx context.Context) error {
	err := c.srv.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start trace service: %v", err)
	}

	go c.wait(ctx)

	return nil
}

// Shutdown stops the collector
func (c *Collector) Shutdown(ctx context.Context) error {
	err := c.srv.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("failed to shutdown trace service: %v", err)
	}

	return nil
}

func (c *Collector) wait(ctx context.Context) {
	log.Println("I! Everything is ready for traces, begin tracing.")

LOOP:
	for {
		select {
		case err := <-c.srv.host.asyncErrorChannel:
			log.Println("E! Asynchronous error received, terminating tracing:", err)
			break LOOP
		case <-ctx.Done():
			log.Println("E! Context done, terminating tracing:", ctx.Err())
			_ = c.Shutdown(context.Background())
		}
	}

	_ = c.Shutdown(ctx)
}
