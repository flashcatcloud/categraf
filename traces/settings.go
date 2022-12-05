//go:build !no_traces

package traces

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.uber.org/zap"
)

// settings holds configuration for building a new service.
type settings struct {
	// Factories component factories.
	Factories component.Factories

	// BuildInfo provides collector start information.
	BuildInfo component.BuildInfo

	// Config represents the configuration of the service.
	Config *config.Config

	// AsyncErrorChannel is the channel that is used to report fatal errors.
	AsyncErrorChannel chan error

	// LoggingOptions provides a way to change behavior of zap logging.
	LoggingOptions []zap.Option
}
