# Jolokia (Shared Library)

该目录存放了 Jolokia 协议的共享客户端代码和采集器逻辑。
它**不是**一个可以直接使用的 Categraf 插件。

直接使用的采集插件为：
- `jolokia_agent`: 适用于直连各个 Java 应用内部署的 Jolokia Agent (推荐)。
- `jolokia_proxy`: 适用于通过 Jolokia Proxy 集中采集多台 Java 应用的场景。

详情请参考上述两个插件的文档。
