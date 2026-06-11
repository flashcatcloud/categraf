# JBoss Monitoring Plugin

Categraf does not require a dedicated native plugin to monitor JBoss (WildFly). Since JBoss runs on the JVM and exposes rich monitoring data via JMX, it is highly recommended to use Categraf's built-in `jolokia_agent` plugin to fetch these metrics.

## Configuration

To monitor JBoss, please configure the `jolokia_agent_misc` plugin directly. We have already prepared a template configuration suitable for JBoss in the example configuration directory.

Please refer to: [jboss.toml](../../conf/input.jolokia_agent_misc/jboss.toml)

Steps:
1. Copy the reference configuration above into your Categraf `conf/input.jolokia_agent_misc/` directory.
2. Ensure that Jolokia Agent is deployed and enabled on your JBoss Server (usually by deploying `jolokia.war`).
3. Modify the `urls` in the configuration file to point to your real Jolokia JMX HTTP Endpoint (e.g., `http://localhost:8080/jolokia/`).

## Metrics and Dashboards

Because the actual metric collection is handled by the Jolokia Agent, the metrics collected depend entirely on the `metrics` blocks defined in your configuration file. Common metrics include JVM memory, thread counts, JBoss Web connections, and sessions.
In your Grafana or Nightingale dashboards, simply query metrics starting with `jolokia_` or whatever `name_prefix` you defined in the configuration.
