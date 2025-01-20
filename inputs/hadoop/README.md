# Hadoop 监控配置文档

本文档详细介绍了如何配置 Hadoop 监控插件，适用于监控 Hadoop 集群中的各个组件（如 Yarn ResourceManager、Yarn NodeManager、Hadoop NameNode 和 Hadoop DataNode）。通过 JMX 接口，可以获取这些组件的详细性能指标。

---

## 配置文件说明

Hadoop 监控插件的配置文件位于 `conf/input.hadoop/hadoop.toml`。配置文件分为两部分：
1. **通用配置**：适用于所有组件的全局配置。
2. **组件配置**：针对每个组件的具体配置。

---

## 通用配置

通用配置适用于所有 Hadoop 组件，主要包括 SASL 认证、Kerberos 认证等。

```toml
[common]
useSASL = false
saslUsername = "HTTP/_HOST"
saslDisablePAFXFast = true
saslMechanism = "gssapi"
kerberosAuthType = "keytabAuth"
keyTabPath = "/path/to/keytab"
kerberosConfigPath = "/path/to/krb5.conf"
realm = "EXAMPLE.COM"
```

### 配置项说明
- `useSASL`：是否启用 SASL 认证。
- `saslUsername`：SASL 认证的用户名。
- `saslDisablePAFXFast`：是否禁用 PA-FX-FAST 机制。
- `saslMechanism`：SASL 认证机制，如 `gssapi`。
- `kerberosAuthType`：Kerberos 认证类型，如 `keytabAuth`。
- `keyTabPath`：Kerberos keytab 文件路径。
- `kerberosConfigPath`：Kerberos 配置文件路径。
- `realm`：Kerberos 域。

---

## 组件配置

每个组件的配置通过 `[[components]]` 块定义。以下是支持的组件及其配置示例。

### 1. Yarn ResourceManager

```toml
[[components]]
name = "YarnResourceManager"
port = 8088
processName = "org.apache.hadoop.yarn.server.resourcemanager.ResourceManager"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    # 集群整体运行状态和资源使用情况
    "NumActiveNMs", # 活跃的NodeManager数量
    "NumUnhealthyNMs", # 不健康的NodeManager数量
    "NumLostNMs", # 丢失连接的NodeManager数量
]
```

### 2. Yarn NodeManager

```toml
[[components]]
name = "YarnNodeManager"
port = 8042
processName = "Dproc_nodemanager"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    # 容器状态指标
    "ContainersLaunched",        # 已启动的容器总数
    "ContainersCompleted",       # 已完成的容器总数
    "ContainersFailed",          # 失败的容器总数
]
```

### 3. Hadoop NameNode

```toml
[[components]]
name = "HadoopNameNode"
port = 50070
processName = "org.apache.hadoop.hdfs.server.namenode.NameNode"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    # NameNode 基本健康状态
    "FSState", # NameNode 文件系统状态(Operational/SafeMode等)
    "HAState", # HA状态(active/standby)
    "State", # NameNode 状态
]
```

### 4. Hadoop DataNode

```toml
[[components]]
name = "HadoopDataNode"
port = 1022
processName = "Dproc_datanode"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    # 系统资源相关
    "SystemCpuLoad",              # 系统CPU负载
    "ProcessCpuLoad",             # DataNode进程CPU负载
    "HeapMemoryUsage",            # JVM堆内存使用情况
]
```

---

## 白名单的作用

- **白名单**：`white_list` 用于指定需要采集的指标名称。插件会根据白名单中的指标名称从 JMX 接口中提取对应的数据。
- **动态采集**：插件会根据 `processName` 判断当前机器是否有该进程，如果有则自动采集白名单中的指标。
- **递归解析**：如果开启 `allowRecursiveParse`，插件会递归解析 JMX 返回的 JSON 数据，并采集白名单中的指标。

---

## 使用方法

1. **配置白名单**：在 `white_list` 中添加需要采集的指标名称。不需要采集的指标可以注释掉。
2. **动态采集**：插件会根据 `processName` 自动判断是否需要采集该组件的指标。
3. **递归解析**：如果需要采集嵌套的 JSON 数据，可以开启 `allowRecursiveParse`。

---

## 示例

以下是一个完整的配置示例：

```toml
[common]
useSASL = false
saslUsername = "HTTP/_HOST"
saslDisablePAFXFast = true
saslMechanism = "gssapi"
kerberosAuthType = "keytabAuth"
keyTabPath = "/path/to/keytab"
kerberosConfigPath = "/path/to/krb5.conf"
realm = "EXAMPLE.COM"

[[components]]
name = "YarnResourceManager"
port = 8088
processName = "org.apache.hadoop.yarn.server.resourcemanager.ResourceManager"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    "NumActiveNMs", # 活跃的NodeManager数量
    "NumUnhealthyNMs", # 不健康的NodeManager数量
    "NumLostNMs", # 丢失连接的NodeManager数量
]

[[components]]
name = "YarnNodeManager"
port = 8042
processName = "Dproc_nodemanager"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    "ContainersLaunched",        # 已启动的容器总数
    "ContainersCompleted",       # 已完成的容器总数
    "ContainersFailed",          # 失败的容器总数
]

[[components]]
name = "HadoopNameNode"
port = 50070
processName = "org.apache.hadoop.hdfs.server.namenode.NameNode"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    "FSState", # NameNode 文件系统状态(Operational/SafeMode等)
    "HAState", # HA状态(active/standby)
    "State", # NameNode 状态
]

[[components]]
name = "HadoopDataNode"
port = 1022
processName = "Dproc_datanode"
allowRecursiveParse = true
allowMetricsWhiteList = true
jmxUrlSuffix = "/jmx"
white_list = [
    "SystemCpuLoad",              # 系统CPU负载
    "ProcessCpuLoad",             # DataNode进程CPU负载
    "HeapMemoryUsage",            # JVM堆内存使用情况
]
```