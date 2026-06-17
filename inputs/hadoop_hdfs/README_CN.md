# Hadoop HDFS 监控插件

Categraf 监控 Hadoop HDFS 时，不需要专门的独立的二进制原生插件。HDFS 提供 JMX 接口暴露监控数据，因此推荐使用 Categraf 自带的 `jolokia_agent` 插件来抓取这些指标。

## 配置说明

要配置 HDFS 的监控，请直接修改 `jolokia_agent` 的配置文件。我们在配置示例目录中已经准备好了一份适用于 Hadoop HDFS 的模板。

请参考：[hadoop-hdfs.toml](../../conf/input.jolokia_agent_misc/hadoop-hdfs.toml)

具体步骤：
1. 将上述参考配置复制到您的 Categraf `conf/input.jolokia_agent_misc/` 目录中。
2. 确保您的 Hadoop NameNode 或 DataNode 启用了 Jolokia Agent。
3. 修改配置文件中的 `urls`，指向真实的 Jolokia JMX HTTP Endpoint (例如: `http://localhost:8778/jolokia/`)。

## 采集指标与大盘

由于实际上使用的是 Jolokia Agent，采集到的指标取决于配置文件中的 `[[instances.metric]]`。当前模板使用 `metrics_name_prefix`，请在 Grafana 或夜莺监控大盘中查询 `hadoop_hdfs_namenode_` 或 `hadoop_hdfs_datanode_` 开头的指标。
