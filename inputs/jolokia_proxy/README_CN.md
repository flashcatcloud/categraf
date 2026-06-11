# Jolokia Proxy 采集插件

该插件通过 Jolokia Proxy 集中采集多台目标 Java 应用程序的 JMX 指标。

如果您有大量的 Java 服务，但在各个业务进程中部署 Jolokia Agent 或打通网络端口存在困难，您可以部署一个集中的 Jolokia Proxy 服务，让 Categraf 通过该 Proxy 代理请求各个目标服务的 JMX 端口。

## 配置说明

```toml
# 采集周期
# interval = 60

[[instances]]
# Jolokia Proxy 服务的访问地址 (只有一个代理服务 URL)
url = "http://localhost:8080/jolokia"

# 访问 Proxy 服务的认证凭据
# username = "proxyadmin"
# password = "proxypassword"

# ===== 目标服务 (Target) 配置 =====
# 默认的访问目标服务的凭据
# default_target_username = "admin"
# default_target_password = "password"

# 配置需要代理采集的目标服务 URL 列表
[[instances.target]]
url = "service:jmx:rmi:///jndi/rmi://target-host-1:9010/jmxrmi"
# username = "custom_user"
# password = "custom_password"

[[instances.target]]
url = "service:jmx:rmi:///jndi/rmi://target-host-2:9010/jmxrmi"

# ===== 指标采集配置 =====
# 与 jolokia_agent 一致，配置您想采集的 MBean
[[instances.metric]]
name  = "java_memory"
mbean = "java.lang:type=Memory"
```

## 采集指标与监控大盘

由于此插件采集的内容与 `jolokia_agent` 一致，指标名称和结构均取决于 `[[instances.metric]]` 的配置。

因此，它没有一个统一的预置大盘，您需要基于具体业务逻辑 (如 Tomcat / JBoss / Kafka) 进行自定义，或复用其他 Jolokia Agent 的大盘。
