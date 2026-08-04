# SNMP Input Plugin

This plugin actively polls monitoring metrics from network devices (e.g., switches, routers, firewalls) that support the SNMP protocol.
It is forked from [telegraf/snmp](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/snmp) and has been adapted and optimized for Categraf's underlying logic (like the netsnmp integration).

## Configuration

You can flexibly collect scalar fields (`field`) or tabular data (`table`) by configuring the respective OIDs.

```toml
# Collect SNMP monitoring data
# interval = 60

[[instances]]
# SNMP Agent addresses
agents = ["udp://172.30.15.189:161"]

# SNMP Timeout and Retries
timeout = "5s"
retries = 1

# SNMP Version, supports 1, 2, 3
version = 2
community = "public"

# (SNMP v3 Configurations, required if version=3)
# sec_name = ""
# sec_level = "authPriv"
# context_name = ""
# auth_protocol = "MD5"
# auth_password = ""
# priv_protocol = "DES"
# priv_password = ""

# Automatically inject the target agent's IP into a specific tag
agent_host_tag = "ident"

# Table collection error policy. The default keeps historical behavior.
# Supported values:
#   legacy: a field error fails the current table
#   partial: keep successfully collected value fields, but fail closed when
#            tag, filter, secondary-index, or inherited-tag dependencies are unknown
# default_table_error_policy = "legacy"

# Dependency cache for partial error policy. The cache only stores dependency
# values such as tags, filter fields, secondary-index mappings, and top-level
# inherited tags. A zero TTL disables it.
# dependency_cache_ttl = "0s"
# dependency_cache_max_entries = 0

# ================================
# Scalar Fields Configuration
# ================================
[[instances.field]]
oid = "RFC1213-MIB::sysUpTime.0"
name = "uptime"

[[instances.field]]
oid = "RFC1213-MIB::sysName.0"
name = "source"
is_tag = true # Extract this field as a Tag instead of a numeric metric

# ================================
# Tables Configuration
# ================================
[[instances.table]]
oid = "IF-MIB::ifTable"
name = "interface"
# Inherit specified Tags from outer fields into all rows of the table
inherit_tags = ["source"]
# Optional per-table override: "legacy" or "partial"
# error_policy = "partial"

[[instances.table.field]]
oid = "IF-MIB::ifDescr"
name = "ifDescr"
is_tag = true

# The same OID returns "34%" normally and "offline" on failure
[[instances.table.field]]
oid = "1.3.6.1.4.1.19046.11.1.1.3.2.1.3"
name = "fan_speed"
conversion = "float"

[[instances.table.field.convert_rule]]
match = "offline"
value = -1
```

`convert_rule` entries are evaluated in configuration order before the field's existing `conversion`, and the first match wins. Rules support exact `match`, Go `regex`, capture expansion through `extract`, fixed `value`, and a rule-level `conversion`. If no rule matches, processing falls back to the field's existing `conversion`. The example maps `offline` to `-1`, while `34%` falls back to `conversion = "float"` and becomes `34`. For top-level fields, use `[[instances.field.convert_rule]]`.

### Table Error Policy

`default_table_error_policy` controls how table collection behaves when a field fails. The default value `legacy` preserves existing behavior: an error in any table field causes the current table to fail. Set it to `partial` when you want Categraf to keep successfully collected value fields from the same table.

`partial` still fails closed for dependencies that affect row identity or tag correctness, including tag fields, filters, secondary indexes, and inherited tags. This avoids emitting values with incomplete or mismatched tags.

`error_policy` can be set under a specific `[[instances.table]]` to override `default_table_error_policy` for that table only.

`dependency_cache_ttl` enables a per-agent dependency cache for `partial` mode. It caches dependency values such as tag fields, filter fields, secondary-index mappings, and inherited top-level tags so a temporary dependency read failure can reuse recent values. The default `0s` disables the cache. `dependency_cache_max_entries` limits cache entries per agent; `0` uses the built-in default.

## Metrics

The names of the collected metrics and tags are entirely determined by the `name` parameters you define in the `field` and `table` sections of the configuration file.
Common network metrics collected typically include:
- `uptime`: Device uptime
- `interface_ifInOctets` / `interface_ifOutOctets`: Port inbound/outbound traffic
- `interface_ifInErrors` / `interface_ifOutErrors`: Port inbound/outbound errors

## Dashboards

Because the SNMP metrics are entirely driven by your custom OID configurations, there is no one-size-fits-all Dashboard.
A basic universal Dashboard is provided in this directory targeted at the classic network interfaces (IF-MIB) shown in the configuration example, mainly used for monitoring port traffic and error packets.
