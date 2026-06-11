# Kafka Connect 监控插件

Categraf 监控 Kafka Connect 时，不需要专门的独立原生插件。Kafka Connect 作为 Java 应用程序，通过 JMX 接口暴露了完整的监控数据，因此推荐使用 Categraf 自带的 `jolokia_agent` 或 `jolokia_agent_misc` 插件来抓取这些指标。

## 配置说明

要配置 Kafka Connect 的监控，请直接修改 `jolokia_agent_misc` 的配置文件。我们在配置示例目录中已经准备好了一份适用于 Kafka Connect 的模板。

请参考：[kafka-connect.toml](../../conf/input.jolokia_agent_misc/kafka-connect.toml)

具体步骤：
1. 将上述参考配置复制到您的 Categraf `conf/input.jolokia_agent_misc/` 目录中。
2. 确保您的 Kafka Connect Worker 节点上启用了 Jolokia Agent。
3. 修改配置文件中的 `urls`，指向真实的 Jolokia JMX HTTP Endpoint (例如: `http://localhost:8778/jolokia/`)。

## 采集指标与大盘

由于实际上使用的是 Jolokia Agent，采集到的指标完全取决于配置文件中配置的 `metrics`。常见的指标包括 Source/Sink Task 的运行状态、提交延迟、吞吐量等。
请在您的 Grafana 或夜莺监控大盘中直接使用对应的 JMX 映射前缀查询指标。
