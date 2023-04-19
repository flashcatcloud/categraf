//go:build !no_traces

package traces

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/otel/metric/nonrecording"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"

	"flashcat.cloud/categraf/pkg/otel/extensions"
	"flashcat.cloud/categraf/pkg/otel/pipelines"
	"flashcat.cloud/categraf/pkg/otel/telemetrylogs"
)

type service struct {
	buildInfo component.BuildInfo
	config    *config.Config
	telemetry component.TelemetrySettings
	host      *serviceHost
}

func newService(set *settings) (srv *service, err error) {
	srv = &service{
		buildInfo: set.BuildInfo,
		config:    set.Config,
		telemetry: component.TelemetrySettings{
			TracerProvider: trace.NewNoopTracerProvider(),
			MeterProvider:  nonrecording.NewNoopMeterProvider(),
		},
		host: &serviceHost{
			factories:         set.Factories,
			buildInfo:         set.BuildInfo,
			asyncErrorChannel: set.AsyncErrorChannel,
		},
	}

	srv.telemetry.Logger, err = telemetrylogs.NewLogger(set.Config.Service.Telemetry.Logs, set.LoggingOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	srv.host.builtExtensions, err = extensions.Build(context.Background(),
		srv.telemetry, srv.buildInfo, srv.config.Extensions, srv.config.Service.Extensions, srv.host.factories.Extensions)
	if err != nil {
		return nil, fmt.Errorf("cannot build tracing extensions: %w", err)
	}

	srv.host.pipelines, err = pipelines.Build(context.Background(),
		srv.telemetry, srv.buildInfo, srv.config, srv.host.factories)
	if err != nil {
		return nil, fmt.Errorf("cannot build tracing pipelines: %w", err)
	}

	return srv, nil
}

func (srv *service) Start(ctx context.Context) error {
	if err := srv.host.builtExtensions.StartAll(ctx, srv.host); err != nil {
		return fmt.Errorf("failed to start tracing extensions: %w", err)
	}

	if err := srv.host.pipelines.StartAll(ctx, srv.host); err != nil {
		return fmt.Errorf("cannot start tracing pipelines: %w", err)
	}

	return srv.host.builtExtensions.NotifyPipelineReady()
}

func (srv *service) Shutdown(ctx context.Context) error {
	// Accumulate errors and proceed with shutting down remaining components.
	var errs error

	if err := srv.host.builtExtensions.NotifyPipelineNotReady(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to notify that tracing pipeline is not ready: %w", err))
	}

	if err := srv.host.pipelines.ShutdownAll(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown tracing pipelines: %w", err))
	}

	if err := srv.host.builtExtensions.ShutdownAll(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown tracing extensions: %w", err))
	}

	// TODO: Shutdown TracerProvider, MeterProvider, and Sync Logger.
	return errs
}
