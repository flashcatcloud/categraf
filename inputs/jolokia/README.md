# Jolokia (Shared Library)

This directory contains the shared client code and gatherer logic for the Jolokia protocol. 
It is **NOT** a standalone Categraf plugin that can be enabled directly.

For the actual plugins, please refer to:
- `jolokia_agent`: Used to directly connect to Jolokia Agents deployed inside individual Java applications (Recommended).
- `jolokia_proxy`: Used to collect metrics from multiple Java applications centrally via a Jolokia Proxy.

Please refer to the documentation in those respective plugin directories for more details.

`PREFER_IPV4_STACK=true` enables IPv4 for this plugin's HTTP connections. Restart Categraf to apply; IPv6-only targets fail without fallback. See the [implementation status and remaining paths](../../docs/issue-1513-plugin-ipv4-implementation.md).
