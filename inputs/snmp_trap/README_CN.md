# SNMP Trap 采集插件

该插件用于接收网络设备主动发出的 SNMP Notifications (包括 Traps 和 Inform 请求)。
通过监听 UDP 端口（默认 162），Categraf 可以接收设备的告警事件，并将其解析转换为时序监控指标。

本插件 fork 自 `telegraf/snmp_trap`，并在其基础上增强了 MIB 解析和指标名称的重映射能力。

## 前置要求

在 Linux 上监听 1024 以下的特权端口（如 162），通常需要 `root` 权限。为了遵循最小权限原则，建议不要直接使用 root 运行，而是通过 `setcap` 赋予 Categraf 二进制文件网络绑定特权：

```shell
setcap cap_net_bind_service=+ep /usr/bin/categraf
```

## 配置说明

```toml
# 接收 SNMP Traps
[[instances]]
# 监听的传输协议、本地地址和端口。留空 IP 表示监听所有网卡。
# service_address = "udp://:162"

# 指定 MIB 文件的加载路径，供 gosmi 引擎进行 OID 翻译使用。
# path = ["/usr/share/snmp/mibs"]

# 使用的解析翻译引擎，推荐使用默认的 "gosmi"
# translator = "gosmi"

# (SNMPv3 配置，如果在设备侧配置了 V3 trap 转发，则需在此配置对应的凭证)
# sec_name = "myuser"
# auth_protocol = "MD5"
# auth_password = "pass"
# sec_level = "authNoPriv"

# =======================================================================
# 指标聚合与映射配置 (Metric Aggregation and Mapping)
# =======================================================================

# 1. 全局字段转标签 (fields_to_labels)
# 如果 Trap 中带有的 Varbind 变量名匹配这些名称，它们将自动从指标值转换为 Labels。
# fields_to_labels = ["ifIndex", "ifAdminStatus", "ifOperStatus"]

# 2. 全局 Varbind OID 重命名映射 (varbind_mapping)
# [instances.varbind_mapping]
#   ".1.3.6.1.2.1.2.2.1.1" = "ifIndex"

# 3. 针对特定 Trap 的映射规则 (trap_mapping)
# 可以针对特定的 Trap OID 设置核心指标名 (name) 及主值 (value) 的提取。
# [[instances.trap_mapping]]
#   oid = ".1.3.6.1.6.3.1.1.5.3"
#   name = "link_down"
#   value = ".1.3.6.1.2.1.1.3"  # 将 sysUpTime 提取为主指标的值
```

## 采集指标

SNMP Trap 默认输出的核心指标名为 `snmp_trap`（或通过 `trap_mapping` 映射的其他名字如 `snmp_trap_link_down`）。

该指标将携带以下核心标签：
- `source`: 发送 Trap 的源 IP
- `name`: 翻译后的 Trap 名称 (如 `linkDown`)
- `oid`: Trap 的 OID
- `mib`: 所在的 MIB 模块名
- `version`: Trap 版本 ("1", "2c" 或 "3")

**关于字段处理：**
Trap 报文内附带的各个 Varbind 将作为这个指标的 fields 或被解析提取为 Tags（如果配置了 `fields_to_labels`）。

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，用于监控 SNMP Trap 的接收总量、Trap 名称分布以及来源 IP 的事件分布情况。
