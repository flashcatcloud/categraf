forked from [telegraf/snmp](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/snmp)

目前只修改了netsnmp的部分 ，配置中为了兼容，保留了path参数。

配置示例
```
[[instances]]
agents = ["udp://172.30.15.189:161"]

timeout = "5s"
version = 2
community = "public"
agent_host_tag = "ident"
retries = 1

[[instances.field]]
oid = "RFC1213-MIB::sysUpTime.0"
name = "uptime"

[[instances.field]]
oid = "RFC1213-MIB::sysName.0"
name = "source"
is_tag = true

[[instances.table]]
oid = "IF-MIB::ifTable"
name = "interface"
inherit_tags = ["source"]

[[instances.table.field]]
oid = "IF-MIB::ifDescr"
name = "ifDescr"
is_tag = true

```