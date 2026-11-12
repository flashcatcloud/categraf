# gNMI (gRPC Network Management Interface) 采集插件

该插件基于 [gNMI](https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md) 协议的 Subscribe 方法，订阅并采集网络设备的遥测 (Telemetry) 数据。
插件支持 TLS 认证和加密，与设备供应商无关，支持任何兼容 gNMI 规范的平台。

该插件 fork 自 `telegraf/inputs.gnmi`。对于 Cisco 设备，它特别针对 Cisco IOS XR (64-bit) 6.5.1, Cisco NX-OS 9.3 以及 Cisco IOS XE 16.12 及以上版本产生的遥测数据进行了优化。

## 配置说明

本地修复使用方式、现场验证记录与待优化项见 [使用与验证说明](USAGE_AND_VALIDATION_CN.md)。当前不提供超时预加载，必要标签同步输出方案暂定、尚未实现。

```toml
# gNMI 遥测插件配置
[[instances]]
  ## gNMI gRPC 服务器的地址和端口
  addresses = ["1.2.3.4:5678"]

  ## 设备的认证凭据
  username = "admin"
  password = "admin"

  ## 请求的 gNMI 编码格式 (可选: "proto", "json", "json_ietf", "bytes")
  encoding = "proto"

  ## 发生故障后重新连接的等待时间
  redial = "10s"

  ## gRPC 的最大消息大小限制，默认 4MB
  max_msg_size = 4194304

  ## TLS 认证配置 (如果设备启用了 TLS)
  # enable_tls = false
  # tls_ca = "/etc/pki/ca.pem"
  # tls_min_version = "TLS12"
  # insecure_skip_verify = true # 跳过证书链和主机名验证

  ## 如果在更新消息中没有前缀路径，是否尝试推断路径标签。
  ## 如果启用，则会使用更新中所有元素的公共路径。
  # guess_path_tag = false

  ## 定义额外的别名，用于将响应的路径映射到 measurement 的名称
  # [instances.aliases]
  #   ifcounters = "openconfig:/interfaces/interface/state/counters"

  ## 配置要订阅的遥测路径
  [[instances.subscription]]
    ## 产生的数据将使用的 measurement 名称 (也就是指标前缀)
    name = "ifcounters"

    ## 订阅的起源(Origin)和路径(Path)
    ## origin 通常指设备实现的 YANG 数据模型，path 是类似于 XPath 的结构路径
    origin = "openconfig-interfaces"
    path = "/interfaces/interface/state/counters"

    ## 订阅模式: "target_defined", "sample" (周期采样), "on_change" (变更时推送)
    subscription_mode = "sample"
    sample_interval = "10s"

  ## 如果你想把某个订阅路径的值作为其他指标的 Tag (标签)，可以使用 tag_subscription
  # [[instances.tag_subscription]]
  #  name = "descr"
  #  origin = "openconfig-interfaces"
  #  path = "/interfaces/interface/state"
  #  subscription_mode = "on_change"
```

## 采集指标

### 用标签订阅替代原始路径 key 标签

在对应的 `[[instances.tag_subscription]]` 内设置 `drop_source_key = true`，
可在标签成功关联后删除其源叶子直接父节点上的同名 key 标签。默认 `false`，保持原有行为。
例如，`/interface/name` 生成 `ifname` 后删除接口的原始 `name`；
`/interface/input/queues/queue/name` 生成 `qename` 后仅删除该 input 队列的原始路径标签。

```toml
[[instances.tag_subscription]]
name = "ifname"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/name"
subscription_mode = "sample"
match = "elements"
elements = ["interface"]
disable_concatenation = true
drop_source_key = true

[[instances.tag_subscription]]
name = "qename"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/input/queues/queue/name"
subscription_mode = "sample"
match = "elements"
elements = ["interface", "queue"]
disable_concatenation = true
drop_source_key = true
```

output 队列另配一段，把第二段 `path` 中的 `input` 改为 `output`。
删除要求路径父节点、来源及 key 选择条件匹配，且标签值与 key 值一致。
标签尚未收到、已清空、存在冲突或值不一致时保留原始 key，其他 key 不受影响。
input/output 的标签关联范围仍由 `elements` 决定，但删除只作用于对应方向的路径。
该选项仍依赖设备上报叶子值，不会直接从 path key 生成自定义标签。
无需为这项删除另外配置 relabel。

每配置一个 `[[instances.subscription]]`，插件就会生成对应的 Measurement。
gNMI `SubscribeResponse` 的 Update 消息中，每个叶子节点 (Leaf) 的值都会转化为指标的值 (Field)，路径键值对会被转化为标签 (Tag)。

## 监控大盘

由于 gNMI 的指标完全依赖于您订阅的 YANG 模型路径，指标名称不固定。因此没有提供统一的默认大盘。您需要根据具体的 `name` 配置在 Grafana/Nightingale 中自定义大盘。

## 故障排查排雷

某些设备 (比如 Juniper) 可能会返回与订阅路径不对应的杂散数据路径。在这种情况下，Categraf 无法确定响应应属哪个 `name`，您会看到 `empty metric-name warning` 警告。

为了避免这个问题，您可以使用 `[instances.aliases]` 将响应路径映射回正确的名称：

```toml
[[instances]]
  addresses = ["..."]

  [instances.aliases]
    memory = "/components"

  [[instances.subscription]]
    name = "memory"
    origin = "openconfig"
    path = "/junos/system/linecard/cpu/memory"
    subscription_mode = "sample"
    sample_interval = "60s"
```

## 内置会话健康指标

实例会话指标为 `gnmi_up`：当前会话收到有效 Update 通知或 sync_response=true 后为 1；
启动未确认、重连或 EOF/RPC/协议错误后为 0。`disable_gnmi_up = true` 可关闭。

在 `[[instances]]` 内、subscription 子表之前配置：

```toml
disable_gnmi_up = false
gnmi_up_interval = "30s"
```

未配置或零值采用 30 秒，负值报配置错误。首次 Gather 输出；状态不变时满周期再输出；
状态变化时在下一次 Gather 提前输出，并重新计时。每台设备独立计时，不增加定时协程。
业务采集不受影响，实际输出受 Gather 周期约束：Gather 为 60 秒时不能每 30 秒输出。
两次 Gather 之间发生并恢复的短暂变化可能无法观察。

up 不保证业务数据新鲜或成功转换；ON_CHANGE 正常静默不判 down，尚未检测到的网络黑洞
也可能保持 up。不额外发送 RPC，不新增静默超时重连。该周期不改变 ICMP 探测和上报。

会话健康支持与业务采集相同的 gRPC target，包括 `dns:///device:50051` 等 resolver URI，
不强制要求普通 host:port。标签包含实例静态标签和 agent_host_tag（默认 agent_host）；
无法提取单一主机时，主机标签使用完整配置目标。已有静态标签保持优先。
完整目标通常由 address 标签标识；若该名称被静态标签或 agent_host_tag 占用，
则额外使用 gnmi_target 保留目标身份。若该名称也被占用，则依次尝试 gnmi_target_1、
gnmi_target_2 等，并考虑实例标签、全局标签在输出名称规范化后的冲突。
重复的相同目标每项健康指标只输出一次；会话状态聚合计算：只要仍有一个已确认的订阅存活，
gnmi_up 就保持为 1，新建或失败的其他订阅不会覆盖正常会话；最后一个已确认会话结束后才变为 0。
业务订阅行为不变；不同实例配置同一目标时使用不同静态标签。
统一输出管线的过滤、重命名和 relabel 规则仍然适用；合并多个目标时应保留目标身份标签。

## 可选内置 ICMP

在 subscription 子表前配置 `enable_icmp = true` 开启；默认关闭。
默认值：`icmp_interval="30s"`、`icmp_timeout="3s"`（含 DNS 的整轮截止时间）、
`icmp_count=3`、`icmp_packet_interval="200ms"`、`icmp_concurrency=10`、
`icmp_privileged=true`。未配置或零值使用默认值；负值、interval<=timeout 或
 timeout<=(count-1)*packet_interval 为配置错误。关闭时忽略未使用的 ICMP 参数，
不执行 DNS 和探测。

| 指标 | 含义 |
|---|---|
| gnmi_icmp_probe_success | 有效完整轮次为 1；未就绪、执行错误、结果过期为 0 |
| gnmi_icmp_up | 有回复为 1，全丢包为 0；仅有效且未过期时输出 |
| gnmi_icmp_rtt_seconds | 成功回复平均 RTT，单位秒；无回复不输出；时钟分辨率可能使有效 RTT 为 0 |
| gnmi_icmp_packet_loss_ratio | 丢包数/发送数，0～1；全丢包为 1；仅有效且未过期时输出 |
| gnmi_icmp_last_probe_timestamp_seconds | 最新一轮完成时间（含执行错误），本地 Unix 秒；首轮未完成为 0 |

发送足量探测包后超时无回复，表示 probe_success=1/up=0/loss=1。
权限、DNS、socket、发送不足等错误，表示 probe_success=0，不输出 up/RTT/loss，
不会沿用上一轮成功结果。结果超过 2*interval+timeout 未更新时也停止输出旧值。
每次 Gather 重复读取最新完整快照，通过 last_probe 判断探测时间。

ICMP 在后台有限 worker 中运行，不阻塞 Gather，不改变 gnmi_up，不触发重连。
跨已初始化实例和本地配置文件，相同规范化主机与完全一致的有效探测配置共享一轮网络探测。
去重键包含包数、超时、包间隔、权限、探测周期和实例并发；不同配置独立探测。
静态标签和会话上报周期不影响去重。各实例仍按每次 Gather 输出自己的完整端点、主机及静态标签，
经过原实例级 relabel；尚未完成、失败和过期快照的输出语义不变。
插件顶层 `icmp_max_concurrency` 控制共享池总并发，默认 10；相同配置组也共同遵守该组的
`icmp_concurrency` 上限。请按独立探测数量配置足够的调度容量。
首轮分散在 [0, interval)，后续按完成时间加 interval 和 ±10% 抖动调度，同主机与配置键不重入。
每轮重新解析 DNS，优先 IPv4、其次 IPv6，各地址族内稳定排序选择一个地址；
不表示全部解析地址均在线，不跨 DNS 别名去重。支持带 zone 的 IPv6 字面量。
ICMP 要求普通 host:数字端口（IPv6 使用方括号）；其他 gRPC target 在启动时告警并跳过探测，
只输出 probe_success=0 和为零的 last_probe 时间，不输出 up/RTT/loss。
对应目标的业务采集和会话健康照常运行，不限制静态标签名称。

raw ICMP 需要运行账户具有相应权限，如 Linux CAP_NET_RAW 或 Windows 相应权限；
`icmp_privileged=false` 请求非特权 ICMP socket，支持程度依赖系统。
运行权限或探测错误仅影响 ICMP 指标，不中止 gNMI。真实环回测试需设置
`GNMI_TEST_ICMP=1` 后运行 `go test ./inputs/gnmi -run TestICMPNativeLoopback -v -count=1`。
普通单测无需 ICMP 权限。上线前另行验证设备 ACL、丢包、IPv6 和部署账户权限。

实现说明：gNMI 使用仓库已有的 `golang.org/x/net/icmp`，直接控制 socket 读写截止时间
与取消关闭；不使用锁定版本 pro-bing 的无界发送重试。任一次发送错误都会使整轮无效。
`icmp_count` 上限为 65535，避免 Echo 序号溢出。

## 可选端点聚合指标

原有 `disable_gnmi_up`、`gnmi_up_interval` 和 ICMP 参数继续配置在 `[[instances]]` 内，
默认值、指标名、端点及主机标签、静态标签、实例 relabel 和上报周期保持不变。
不同实例使用同一端点时，应保留可区分实例的静态标签。

在插件顶层、第一个实例之前设置 `enable_endpoint_up = true`，额外输出 `gnmi_endpoint_up`。
该指标跨已初始化实例和本地文件按完整端点聚合：任意会话已确认且仍存活时为 1，否则为 0。
实例的 `gnmi_up=0` 仍会独立输出，不会被该汇总指标覆盖。
实例 `disable_gnmi_up` 只关闭实例指标，不关闭已显式启用的端点聚合。

`endpoint_up_interval` 默认 30 秒，启用时拒绝负值；首次插件 Gather 和观察到的状态变化立即上报，
稳定状态按周期上报，实际频率受插件 Gather 周期约束。聚合有独立的上报计时，
只使用插件级及全局标签和 relabel。端点身份通常使用 address；发生配置或规范化名称冲突时，
选择未占用的 gnmi_target、gnmi_target_1 等名称。
聚合在线不表示每条订阅或每项业务指标均正常。HTTP 下发的独立插件对象及不同进程分别聚合。
默认不开启聚合，不增加新指标。

## 完整配置案例

以下案例分别使用；请按设备调整地址、认证、TLS、origin 和订阅路径。实例参数必须放在
`[[instances.subscription]]` 或 `[[instances.tag_subscription]]` 子表之前。

### 案例一：仅开启 gnmi_up，稳定状态每 30 秒上报

```toml
interval = "1s"

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = false
# username = "your-user"
# password = "your-password"

[[instances.subscription]]
name = "ifcounters"
origin = "openconfig-interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"
```

只输出 `gnmi_up` 这一项会话指标。首次及状态变化在 Gather 时输出；状态稳定时每 30 秒
输出一次。业务订阅仍按设备接受的采样周期运行，Gather 每秒转发收到的数据。

### 案例二：gnmi_up 加可选 ICMP

```toml
interval = "1s"

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = true
icmp_interval = "30s"
icmp_timeout = "3s"
icmp_count = 3
icmp_packet_interval = "200ms"
icmp_concurrency = 10
icmp_privileged = true

[[instances.subscription]]
name = "ifcounters"
origin = "openconfig-interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"
```

ICMP 需要对应 socket 权限；探测失败不阻止 gNMI。`icmp_interval` 控制后台探测频率，
不控制输出频率：ICMP 最新结果仍随每次 Gather 输出。`gnmi_up_interval` 仅控制 gnmi_up。

### 案例三：通过标签订阅替代接口和队列的源 key

以下是独立的完整配置；地址、认证、TLS、origin 和路径按设备调整。
案例使用设备需实际支持的 RDMA 队列路径，路径存在与否须按型号核对。

```toml
interval = "1s"

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
# username = "your-user"
# password = "your-password"

[[instances.subscription]]
name = "rdma_input"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/input/queues/queue/state"
subscription_mode = "sample"
sample_interval = "15s"

[[instances.tag_subscription]]
name = "ifname"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/name"
subscription_mode = "sample"
sample_interval = "15s"
match = "elements"
elements = ["interface"]
disable_concatenation = true
drop_source_key = true

[[instances.tag_subscription]]
name = "qename"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/input/queues/queue/name"
subscription_mode = "sample"
sample_interval = "15s"
match = "elements"
elements = ["interface", "queue"]
disable_concatenation = true
drop_source_key = true
```

关联成功且值一致后，接口的原始 name 标签由 ifname 替代，input 队列的原始路径 key 标签
由 qename 替代；其他 key（如 id）保留。标签尚未收到、来源不匹配、存在冲突或值不一致时，
原 key 保留。drop_source_key 必须在每段需要替换的 tag_subscription 内单独开启，默认 false。
output 队列需增加对应业务订阅和标签订阅，将 input 改为 output。
与 gnmi_up / ICMP 一起使用时，它们的实例级选项必须放在任何订阅子表之前。

### 案例四：保留实例健康并额外开启端点聚合

请按设备调整路径和认证。两个实例的有效 ICMP 参数相同，共享同一轮探测。
若 CPU 会话断开但接口会话仍在线，各自 gnmi_up 分别为 0 和 1，gnmi_endpoint_up 仍为 1。

```toml
interval = "1s"
enable_endpoint_up = true
endpoint_up_interval = "30s"
icmp_max_concurrency = 10

[[instances]]
addresses = ["192.0.2.10:50051"]
labels = { subscription = "interfaces" }
enable_icmp = true
[[instances.subscription]]
name = "interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"

[[instances]]
addresses = ["192.0.2.10:50051"]
labels = { subscription = "cpu" }
enable_icmp = true
[[instances.subscription]]
name = "cpu"
path = "/system/cpus/cpu/state"
subscription_mode = "sample"
sample_interval = "15s"
```
