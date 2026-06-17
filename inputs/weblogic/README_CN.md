# WebLogic 采集插件 (基于 Jolokia)

WebLogic 的原生监控数据通常暴露在 JMX 中。在 Categraf 中，我们并没有专门开发一个原生的 WebLogic Go 插件，而是推荐使用通用的 **Jolokia** 方式来采集。

## 采集方法

WebLogic 当前可以使用 `jolokia_agent` 插件来监控，通过 HTTP 请求读取 Jolokia 代理暴露出的 JMX 数据，从而获取 WebLogic 的各种监控指标（如 JVM 内存、线程池、JDBC 连接池等）。

具体配置和预置的 WebLogic JMX 采集项，请直接参考我们提供的示例配置文件：
[weblogic.toml](../../conf/input.jolokia_agent_misc/weblogic.toml)

## 监控大盘

既然数据是通过 `jolokia_agent` 采集的，所有的指标和标签体系将遵循 Jolokia 规范。
本目录下提供了一个适配当前 WebLogic Jolokia 示例配置的基础 `dashboard.json`。如果你还需要更完整的 JVM 监控，也可以配合使用 `jolokia` 或 `jvm` 相关的通用 Dashboard。
