# Bitbucket

Bitbucket can be monitored using the `jolokia_agent` plugin, which retrieves metrics by reading JMX data from Atlassian Bitbucket.

For configuration details, please refer to: [bitbucket.toml](../../conf/input.jolokia_agent_misc/bitbucket.toml).

## Metrics

Once configured via the Jolokia Agent plugin, Categraf will export the following types of metrics:
- **JVM Metrics**: e.g., `bitbucket_jvm_operatingsystem_*`, `bitbucket_jvm_memory_*`, `bitbucket_jvm_thread_*`
- **Webhooks**: e.g., `bitbucket_webhooks_*`
- **Atlassian Bitbucket Metrics**: e.g., `bitbucket_atlassian_*`
- **Thread Pools**: e.g., `bitbucket_thread_pools_*`
