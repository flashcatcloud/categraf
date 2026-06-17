# ActiveMQ

ActiveMQ can be monitored using the `jolokia_agent` plugin, which retrieves metrics by reading JMX data. 

For configuration details, please refer to: [activemq.toml](../../conf/input.jolokia_agent_misc/activemq.toml).

## Metrics

Once configured via the Jolokia Agent plugin, Categraf will export the following types of metrics:
- **Broker Metrics**: e.g., `activemq_broker_TotalMessageCount`, `activemq_broker_TotalConsumerCount`
- **Queue Metrics**: e.g., `activemq_queue_QueueSize`, `activemq_queue_ConsumerCount`
- **Topic Metrics**: e.g., `activemq_topic_EnqueueCount`, `activemq_topic_DequeueCount`
- **JVM Metrics**: Generic Java Runtime metrics such as Garbage Collection, Memory Heap, etc.
