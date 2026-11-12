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

本地 gNMI 插件跨文件、跨实例按配置的 `address`（host:port）汇总，每个地址仅输出一条
`gnmi_up`。该地址的已初始化实例中，只要有一条会话收到有效数据通知或同步响应且仍在线，
就报 1；所有参与会话均未确认或已断开时，报 0。

将上报开关和周期与采集间隔一起设置在插件顶层：

```toml
interval = "1s"
disable_gnmi_up = false
gnmi_up_interval = "30s"
```

三个字段在整个目录各定义一次，并放在第一个 `[[instances]]` 之前。拆分文件时可放在
排序靠前的 `00-gnmi.toml`，本地加载器按文件名顺序拼接 TOML。请把原实例级健康选项迁移
到插件顶层，旧值不再控制上报。

首次插件 Gather 上报；状态变化在下一次 Gather 提前上报并重新计时，状态稳定则按
`gnmi_up_interval` 上报。未配置或为零时默认 30 秒，负值报错。实际间隔受插件 Gather
周期约束，两次 Gather 之间恢复的短暂故障可能不会反映在指标上。

指标使用端点地址及插件级、全局标签处理，不继承实例级标签或 relabel 规则。
若插件标签占用 address，端点身份依次选择未占用的 gnmi_target、gnmi_target_1 等名称，
并避让插件级、全局标签规范化后的名称，保留已有标签。请保证最终身份标签仍能区分不同地址。
HTTP 下发的独立插件对象和不同 Categraf 进程分别汇总。业务采集仍按实例配置运行。

## 可选内置 ICMP

ICMP 默认关闭。请在第一个 `[[instances]]` 之前，与 `interval` 一起在插件顶层
显式设置 `enable_icmp = true`。请迁移原实例级配置，旧字段仍可解析，但不再生效，
非默认值会触发迁移提示。

后台探测从已初始化实例收集主机，跨文件、跨端口去重。无法提取 host:port 的目标
会跳过，不影响业务采集。按配置的主机名或 IP 去重，DNS 别名不自动合并。

默认探测间隔 30 秒，单轮超时 3 秒（包含 DNS），每轮 3 包，包间隔 200ms，
最大并发 10，特权模式开启。特权探测需要相应的 socket 权限。
`icmp_report_interval` 默认 60 秒。首次结果及可达、不可达、执行失败之间的变化
在下一次 Gather 上报。状态稳定时，到期只上报最新的新一轮结果。结果过期时只上报一次
执行状态异常，不重复发送旧的在线状态、RTT 或丢包率。

ICMP 指标默认按 `icmp_host` 标识主机。若插件标签占用该名称，或全局 `icmp-host` 等别名
规范化后会与其重名，则依次选择未占用的 `gnmi_icmp_host`、`gnmi_icmp_host_1` 等名称，
保留已有标签。与正常全局标签优先级一致，全局同名 `icmp_host` 默认值不会覆盖已生成的主机身份。
每轮上报 `gnmi_icmp_probe_success` 和
`gnmi_icmp_last_probe_timestamp_seconds`。执行成功时还上报 `gnmi_icmp_up` 和
`gnmi_icmp_packet_loss_ratio`，收到回复时才有 `gnmi_icmp_rtt_seconds`。
正常执行但全丢包时，probe_success 为 1、up 为 0、丢包率为 1。执行失败或结果过期
时，probe_success 为 0，省略在线状态、RTT 和丢包率。旧看板请从按 `address` 查询
ICMP 改为按 `icmp_host` 查询；存在标签冲突时，使用实际生成的备用身份标签查询。

## 完整配置案例

以下案例分别使用；请按设备调整地址、认证、TLS、origin 和订阅路径。实例参数必须放在
`[[instances.subscription]]` 或 `[[instances.tag_subscription]]` 子表之前。

### 案例一：仅开启 gnmi_up，稳定状态每 30 秒上报

```toml
interval = "1s"
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = false

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
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
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = true
icmp_interval = "30s"
icmp_timeout = "3s"
icmp_count = 3
icmp_report_interval = "60s"

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"

[[instances.subscription]]
name = "ifcounters"
origin = "openconfig-interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"
```

ICMP 需要对应 socket 权限。探测持续进行，上报只使用尚未发送的新结果。

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
