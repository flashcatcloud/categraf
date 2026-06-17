# JBoss 监控插件

Categraf 监控 JBoss (WildFly) 时，不需要专门的独立原生插件。JBoss 运行在 JVM 之上，通过 JMX 接口可以获取到丰富的监控数据，因此推荐使用 Categraf 自带的 `jolokia_agent` 插件来抓取这些指标。

## 配置说明

要配置 JBoss 的监控，请直接使用并修改 `jolokia_agent_misc` 插件。我们在配置示例目录中已经准备好了一份适用于 JBoss 的模板。

请参考：[jboss.toml](../../conf/input.jolokia_agent_misc/jboss.toml)

具体步骤：
1. 将上述参考配置复制到您的 Categraf `conf/input.jolokia_agent_misc/` 目录中。
2. 确保您的 JBoss 服务器上部署并启用了 Jolokia Agent (通常是部署 jolokia.war)。
3. 修改配置文件中的 `urls`，指向真实的 Jolokia JMX HTTP Endpoint (例如: `http://localhost:8080/jolokia/`)。

## 采集指标与大盘

由于实际上使用的是 Jolokia Agent，采集到的指标取决于配置文件中的 `[[instances.metric]]`。当前模板使用 `metrics_name_prefix = "jboss_"`，请在 Grafana 或夜莺监控大盘中查询 `jboss_` 开头的指标。
