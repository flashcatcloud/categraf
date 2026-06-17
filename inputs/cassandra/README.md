# Cassandra

Cassandra can be monitored using the `jolokia_agent` plugin by reading JMX metrics exposed by the Apache Cassandra JVM. 

For configuration details, please refer to: [cassandra.toml](../../conf/input.jolokia_agent_misc/cassandra.toml).

## Metrics

When configured via the Jolokia Agent plugin, Categraf will export the following metrics:
- **JVM Memory & GC**: e.g., `java_Memory_*`, `java_GarbageCollector_*`
- **Cassandra Cache**: e.g., `cassandra_Cache_*`
- **Cassandra Client & Requests**: e.g., `cassandra_Client_*`, `cassandra_ClientRequest_*`
- **Cassandra Storage & Compaction**: e.g., `cassandra_Storage_*`, `cassandra_Compaction_*`
- **Cassandra Column Family**: e.g., `cassandra_ColumnFamily_*`
