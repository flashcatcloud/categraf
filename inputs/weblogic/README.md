# WebLogic Input Plugin (via Jolokia)

Native monitoring data for WebLogic is typically exposed through JMX. In Categraf, instead of developing a dedicated native WebLogic Go plugin, we strongly recommend using the universal **Jolokia** approach.

## How to Monitor

WebLogic can be monitored using the `jolokia_agent` plugin. It fetches WebLogic metrics (such as JVM memory, thread pools, JDBC connection pools, etc.) by reading JMX data exposed over HTTP by the Jolokia agent.

For specific configurations and pre-defined WebLogic JMX metrics collection items, please refer directly to our provided example configuration file:
[weblogic.toml](../../conf/input.jolokia_agent_misc/weblogic.toml)

## Dashboards

Since the data is collected via `jolokia_agent`, all metrics and tagging systems will follow the Jolokia standards.
A placeholder `dashboard.json` is provided in this directory. For actual JVM monitoring dashboards, it is recommended to use the generic Dashboards associated with `jolokia` or `jvm`.
