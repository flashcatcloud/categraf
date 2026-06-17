# SNMP 采集插件

该插件用于主动拉取支持 SNMP 协议的网络设备（如交换机、路由器、防火墙等）的监控指标。
它从 [telegraf/snmp](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/snmp) fork 而来，并针对 Categraf 的底层逻辑（如 netsnmp 的集成）做出了适配与优化。

## 配置说明

通过配置 OID，可以灵活采集标量字段（`field`）或表格类（`table`）数据。

```toml
# 采集 SNMP 监控数据
# interval = 60

[[instances]]
# SNMP Agent 地址
agents = ["udp://172.30.15.189:161"]

# SNMP 超时与重试
timeout = "5s"
retries = 1

# SNMP 版本，支持 1, 2, 3
version = 2
community = "public"

# (SNMP v3 相关配置，若 version=3 时填写)
# sec_name = ""
# sec_level = "authPriv"
# context_name = ""
# auth_protocol = "MD5"
# auth_password = ""
# priv_protocol = "DES"
# priv_password = ""

# 自动将目标 agent 的 IP 注入到指定标签中
agent_host_tag = "ident"

# ================================
# 标量字段 (Scalar Fields) 配置
# ================================
[[instances.field]]
oid = "RFC1213-MIB::sysUpTime.0"
name = "uptime"

[[instances.field]]
oid = "RFC1213-MIB::sysName.0"
name = "source"
is_tag = true # 将该字段作为 Tag 提取，而不是数值指标

# ================================
# 表格 (Tables) 配置
# ================================
[[instances.table]]
oid = "IF-MIB::ifTable"
name = "interface"
# 从外层字段中继承指定的 Tag 到表内的所有行中
inherit_tags = ["source"]

[[instances.table.field]]
oid = "IF-MIB::ifDescr"
name = "ifDescr"
is_tag = true
```

## 采集指标

所有采集的指标名和标签完全由你在配置文件中的 `field` 和 `table` 中的 `name` 参数决定。
通常采集的通用网络指标包括：
- `uptime`: 设备运行时间
- `interface_ifInOctets` / `interface_ifOutOctets`: 端口进出流量
- `interface_ifInErrors` / `interface_ifOutErrors`: 端口错包数

## 监控大盘

由于 SNMP 采集内容由您的自定义 OID 配置完全决定，因此不存在固定普适的 Dashboard。
本目录下为您提供了一个针对上述配置中经典网络接口 (IF-MIB) 指标的通用基础面板，主要用于监控端口流量和错误包。