# Jolokia Agent Input Plugin

This plugin collects Java JMX metrics by connecting to the Jolokia Agent (`jolokia.war` or `javaagent:jolokia-jvm.jar`) embedded in the target Java application, querying data via Jolokia's HTTP JSON API.

This is the most common way to collect Java application metrics over the network. While direct JMX is also possible, Jolokia is often preferred due to its firewall-friendly nature and lower overhead.

## Configuration

```toml
# Collection interval
# interval = 60

[[instances]]
# URLs of the Jolokia Agents. Multiple URLs are supported.
urls = ["http://localhost:8080/jolokia"]

# Basic Auth credentials (if Jolokia is secured)
# username = "admin"
# password = "password"

# HTTP Request Timeout
# response_timeout = "5s"

# ===== Metrics Collection Configuration =====
# You can define multiple [[instances.metric]] blocks, each mapping to a set of JMX MBean attributes.
[[instances.metric]]
# Prefix for the generated metric names
name  = "java_memory"
# The JMX MBean ObjectName to query
mbean = "java.lang:type=Memory"
# List of attributes to fetch. If empty, fetches all attributes.
# paths = ["HeapMemoryUsage", "NonHeapMemoryUsage", "ObjectPendingFinalizationCount"]

[[instances.metric]]
name  = "java_garbage_collector"
mbean = "java.lang:name=*,type=GarbageCollector"
# Use specific MBean property values as tags
tag_keys = ["name"]
```

## Metrics

The metric names and structures depend entirely on your `[[instances.metric]]` configurations. By default:
- The measurement/metric name will use the specified `name` prefix.
- JMX MBean attributes will be mapped as fields (e.g., `java_memory_HeapMemoryUsage_used`).
- Any property defined in `tag_keys` will be attached as tags to the data points.

## Dashboards

Because the metrics collected via Jolokia are highly customizable (can be used to monitor Tomcat, JBoss, Kafka, or any custom business JMX metrics), you will need to build your own dashboards in Grafana or Nightingale based on the `name` prefixes and specific scenarios you configured.

If you are using our provided example configurations (such as `tomcat.toml`, `kafka.toml`, `activemq.toml`, etc.), you can directly import the default dashboards provided for those specific components.
