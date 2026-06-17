# RocketMQ Offset 采集插件

该插件通过调用 [RocketMQ Dashboard (原 RocketMQ Console)](https://github.com/apache/rocketmq-dashboard) 的 HTTP API 来采集 RocketMQ 各个 Topic 和 Consumer Group 的消费偏移量 (Offset)、消息积压数 (Lag/Diff) 等指标。

这是一种非侵入式采集，适用于没有直接暴露出 OTLP/Prometheus 接口的较老版本 RocketMQ 集群。

## 前置要求

您需要预先部署好 RocketMQ Dashboard 组件，并确保 Categraf 可以访问该组件的 HTTP 端口。

## 配置说明

```toml
# 采集 RocketMQ 消费组积压与偏移量
# interval = 60

[[instances]]
# RocketMQ Dashboard 的访问地址和端口 (无需带 http://)
rocketmq_console_ip_port = "127.0.0.1:8080"

# (可选) 需要忽略采集的 Topic 列表，减少不必要的请求和指标基数
# ignored_topics = ["RETRY_GROUP_TOPIC", "DLQ_GROUP_TOPIC"]
```

## 采集指标

该插件默认会抓取所有的 Topic 列表，然后查询每个 Topic 关联的消费组详情，核心采集指标包括：

- `rocketmq_offset_diff`: 消费组在某个 Broker/Queue 上的消息积压量 (Lag)
- `rocketmq_offset_broker_offset`: Broker 端的最大偏移量
- `rocketmq_offset_consumer_offset`: Consumer 端的已消费偏移量

这些指标将自动携带 `topic`, `consumerGroup`, `brokerName`, `queueId` 等标签。

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，可用于核心的 RocketMQ 消费积压 (Lag) 监控以及整体的偏移量情况观测。
