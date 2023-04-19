//go:build !no_traces

package traces

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"

	"flashcat.cloud/categraf/pkg/otel/extensions"
	"flashcat.cloud/categraf/pkg/otel/pipelines"
)

var _ component.Host = (*serviceHost)(nil)

type serviceHost struct {
	asyncErrorChannel chan error
	factories         component.Factories
	buildInfo         component.BuildInfo

	pipelines       *pipelines.Pipelines
	builtExtensions *extensions.BuiltExtensions
}

// ReportFatalError is used to report to the host that the receiver encountered
// a fatal error (i.e.: an error that the instance can't recover from) after
// its start function has already returned.
func (host *serviceHost) ReportFatalError(err error) {
	host.asyncErrorChannel <- err
}

func (host *serviceHost) GetFactory(kind component.Kind, componentType config.Type) component.Factory {
	switch kind {
	case component.KindReceiver:
		return host.factories.Receivers[componentType]
	case component.KindProcessor:
		return host.factories.Processors[componentType]
	case component.KindExporter:
		return host.factories.Exporters[componentType]
	case component.KindExtension:
		return host.factories.Extensions[componentType]
	}
	return nil
}

func (host *serviceHost) GetExtensions() map[config.ComponentID]component.Extension {
	return host.builtExtensions.GetExtensions()
}

func (host *serviceHost) GetExporters() map[config.DataType]map[config.ComponentID]component.Exporter {
	return host.pipelines.GetExporters()
}
