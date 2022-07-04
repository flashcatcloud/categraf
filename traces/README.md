# traces
Categraf simply wrapped the OpenTelemetry Collector, which means you can get a full support for recving data from and exporting to popular trace vendors, such as the Jaeger and Zipkin.

We only support the common [components](../config/traces/components.go) as default. If you want more, simply add the new ones to [components.go](../config/traces/components.go),
and make sure you configure that in the conf. 

For more details, see the official docs:
- https://opentelemetry.io/docs/collector/getting-started
- https://github.com/open-telemetry/opentelemetry-collector

## Configuration

Here is the [examples](../conf/traces.yaml).