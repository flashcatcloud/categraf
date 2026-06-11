# RocketMQ Offset Input Plugin

This plugin collects metrics for message offsets, broker limits, and consumer lags (diffs) across RocketMQ Topics and Consumer Groups by querying the HTTP API of the [RocketMQ Dashboard (formerly RocketMQ Console)](https://github.com/apache/rocketmq-dashboard).

This provides a non-intrusive way of gathering metrics, particularly useful for older RocketMQ clusters that do not expose native OTLP/Prometheus endpoints.

## Prerequisites

You must have the RocketMQ Dashboard component deployed, and your Categraf instance must have network access to its HTTP port.

## Configuration

```toml
# Collect RocketMQ consumer offset and lag
# interval = 60

[[instances]]
# The host and port of the RocketMQ Dashboard (without http:// prefix)
rocketmq_console_ip_port = "127.0.0.1:8080"

# (Optional) List of Topics to ignore. Useful for reducing unnecessary requests and metric cardinality.
# ignored_topics = ["RETRY_GROUP_TOPIC", "DLQ_GROUP_TOPIC"]
```

## Metrics

By default, the plugin fetches the list of all Topics and then queries the consumer details for each Topic. Key metrics collected include:

- `rocketmq_offset_diff`: The message backlog (lag) for a consumer group on a specific Broker/Queue
- `rocketmq_offset_broker_offset`: The maximum message offset at the Broker
- `rocketmq_offset_consumer_offset`: The offset up to which the Consumer has consumed

These metrics are automatically tagged with `topic`, `consumerGroup`, `brokerName`, and `queueId` labels.

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to monitor critical RocketMQ consumer lags and observe overall offset states.
