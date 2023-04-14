//go:build !no_traces

package traces

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/alibabacloudlogserviceexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/jaegerexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/zipkinexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/spanprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

// Add more factories here if you need
func components() (component.Factories, error) {
	extensions, err := component.MakeExtensionFactoryMap(
		basicauthextension.NewFactory(),
		healthcheckextension.NewFactory(),
		pprofextension.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	receivers, err := component.MakeReceiverFactoryMap(
		jaegerreceiver.NewFactory(),
		zipkinreceiver.NewFactory(),
		otlpreceiver.NewFactory(),
		kafkareceiver.NewFactory(),
		prometheusreceiver.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	exporters, err := component.MakeExporterFactoryMap(
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		jaegerexporter.NewFactory(),
		zipkinexporter.NewFactory(),
		kafkaexporter.NewFactory(),
		alibabacloudlogserviceexporter.NewFactory(),
		prometheusremotewriteexporter.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	processors, err := component.MakeProcessorFactoryMap(
		batchprocessor.NewFactory(),
		attributesprocessor.NewFactory(),
		tailsamplingprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
		spanprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
	)
	if err != nil {
		return component.Factories{}, err
	}

	return component.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}, nil
}
