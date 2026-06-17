# Jolokia Agent 采集插件

该插件用于连接目标 Java 应用程序内嵌部署的 Jolokia Agent (`jolokia.war` 或 `javaagent:jolokia-jvm.jar`)，通过 Jolokia 的 HTTP JSON API 采集 Java JMX 指标。

这是通过网络采集 Java 应用程序指标最常用的方式。对于直接暴露了 JMX 端口的 Java 程序，虽然也可以用 jmx 采集，但 Jolokia 方式往往更容易穿透防火墙且开销更小。

## 配置说明

```toml
# 采集周期
# interval = 60

[[instances]]
# Jolokia Agent 的访问地址，支持配置多个地址
urls = ["http://localhost:8080/jolokia"]

# Basic Auth 认证 (如果目标 Jolokia 配置了认证)
# username = "admin"
# password = "password"

# HTTP 请求超时时间
# response_timeout = "5s"

# ===== 指标采集配置 =====
# 可以配置多个 [[instances.metric]] 块，每个块对应一组 JMX MBean 属性
[[instances.metric]]
# 生成的 metric 前缀
name  = "java_memory"
# JMX MBean 的 ObjectName
mbean = "java.lang:type=Memory"
# 需要采集的属性列表。如果不填则采集该 MBean 的所有属性。
# paths = ["HeapMemoryUsage", "NonHeapMemoryUsage", "ObjectPendingFinalizationCount"]

[[instances.metric]]
name  = "java_garbage_collector"
mbean = "java.lang:name=*,type=GarbageCollector"
# 使用 MBean 中的属性值作为生成的 tags
tag_keys = ["name"]
```

## 采集指标

指标的名称和结构完全取决于 `[[instances.metric]]` 中配置的内容。默认情况下：
- Measurement / Metric 名称会带上 `name` 前缀。
- JMX MBean 的属性将被映射为对应的 Field (如 `java_memory_HeapMemoryUsage_used`)。
- 配置了 `tag_keys` 的属性，将作为 Tag 附加在数据中。

## 监控大盘

由于 Jolokia 采集的指标高度自定义（可采集 Tomcat, JBoss, Kafka, 乃至任何自定义业务的 JMX 指标），您需要根据您配置的 `name` 前缀和具体业务场景，在 Grafana 或夜莺中构建自己的监控大盘。

如果您使用的是我们提供的示例配置（如 `tomcat.toml`, `kafka.toml`, `activemq.toml` 等），则可以直接导入对应组件的默认大盘。
