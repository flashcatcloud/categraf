# Kafka Connect Monitoring Plugin

Categraf does not require a dedicated native plugin to monitor Kafka Connect. Since Kafka Connect is a Java application and exposes comprehensive monitoring data via JMX, it is highly recommended to use Categraf's built-in `jolokia_agent` or `jolokia_agent_misc` plugin to fetch these metrics.

## Configuration

To monitor Kafka Connect, please configure the `jolokia_agent_misc` plugin directly. We have already prepared a template configuration suitable for Kafka Connect in the example configuration directory.

Please refer to: [kafka-connect.toml](../../conf/input.jolokia_agent_misc/kafka-connect.toml)

Steps:
1. Copy the reference configuration above into your Categraf `conf/input.jolokia_agent_misc/` directory.
2. Ensure that Jolokia Agent is enabled on your Kafka Connect Worker node.
3. Modify the `urls` in the configuration file to point to your real Jolokia JMX HTTP Endpoint (e.g., `http://localhost:8778/jolokia/`).

## Metrics and Dashboards

Because the actual metric collection is handled by the Jolokia Agent, the metrics collected depend entirely on the `metrics` blocks defined in your configuration file. Common metrics include Source/Sink Task status, commit latency, and throughput.
In your Grafana or Nightingale dashboards, simply query the mapped JMX metrics prefix defined in your configuration.
