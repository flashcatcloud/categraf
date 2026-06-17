# Jolokia Proxy Input Plugin

This plugin collects JMX metrics from multiple target Java applications by using a centralized Jolokia Proxy.

If you have a large number of Java services and deploying a Jolokia Agent or opening network ports on every single instance is difficult, you can deploy a centralized Jolokia Proxy service. Categraf can then issue requests to the Proxy, which forwards them to the target JMX endpoints.

## Configuration

```toml
# Collection interval
# interval = 60

[[instances]]
# The URL of the Jolokia Proxy service (Only one proxy URL per instance)
url = "http://localhost:8080/jolokia"

# Credentials for accessing the Proxy service itself
# username = "proxyadmin"
# password = "proxypassword"

# ===== Target Configurations =====
# Default credentials for accessing target services
# default_target_username = "admin"
# default_target_password = "password"

# List of target JMX URLs to proxy requests to
[[instances.target]]
url = "service:jmx:rmi:///jndi/rmi://target-host-1:9010/jmxrmi"
# username = "custom_user"
# password = "custom_password"

[[instances.target]]
url = "service:jmx:rmi:///jndi/rmi://target-host-2:9010/jmxrmi"

# ===== Metrics Collection Configuration =====
# Identical to jolokia_agent, configure the MBeans you want to collect
[[instances.metric]]
name  = "java_memory"
mbean = "java.lang:type=Memory"
```

## Metrics and Dashboards

Because this plugin collects the exact same kind of data as the `jolokia_agent` plugin, the metric names and structure depend entirely on your `[[instances.metric]]` configurations.

Therefore, it does not come with a single predefined dashboard. You must customize your dashboard based on the specific business logic (e.g., Tomcat / JBoss / Kafka) you are querying, or reuse existing Jolokia Agent dashboards.
