# Hadoop HDFS Monitoring Plugin

Categraf does not require a dedicated, standalone native plugin to monitor Hadoop HDFS. HDFS exposes its monitoring data via JMX, so it is highly recommended to use Categraf's built-in `jolokia_agent` plugin to fetch these metrics.

## Configuration

To monitor HDFS, please configure the `jolokia_agent` plugin directly. We have already prepared a template configuration suitable for Hadoop HDFS in the example configuration directory.

Please refer to: [hadoop-hdfs.toml](../../conf/input.jolokia_agent_misc/hadoop-hdfs.toml)

Steps:
1. Copy the reference configuration above into your Categraf `conf/input.jolokia_agent_misc/` directory.
2. Ensure that Jolokia Agent is enabled on your Hadoop NameNode or DataNode.
3. Modify the `urls` in the configuration file to point to your real Jolokia JMX HTTP Endpoint (e.g., `http://localhost:8778/jolokia/`).

## Metrics and Dashboards

Because the actual metric collection is handled by the Jolokia Agent, the metrics collected depend entirely on the `metrics` blocks defined in your configuration file. In your Grafana or Nightingale dashboards, simply query metrics starting with `jolokia_` or whatever `name_prefix` you defined in the configuration.
