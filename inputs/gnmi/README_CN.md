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

每次 Gather 均输出健康指标，业务流断开也继续输出。配置
`disable_gnmi_up = true` 可关闭以下全部会话指标：

| 指标 | 含义 |
|---|---|
| gnmi_up | 当前会话收到有效 Update 通知或 sync_response=true 后为 1；启动未确认、重连或 EOF/RPC/协议错误后为 0 |
| gnmi_last_update_timestamp_seconds | 最近包含 Update/Delete 数据通知的本地接收 Unix 秒，包含 tag 数据；从未收到为 0 |
| gnmi_reconnects_total | 首次尝试之后实际开始的连接尝试次数，实例重启归零 |

重连保留历史接收时间。up 不保证业务数据新鲜或成功转换；ON_CHANGE 正常静默
不判 down，尚未检测到的网络黑洞也可能暂时保持 up。本功能不额外发送 RPC，
不新增静默超时重连。

标签包含实例静态标签、agent_host_tag（默认 agent_host）和完整端点 address。
实例静态标签不得占用这两个身份标签，agent_host_tag 不得设为 address。
地址必须是唯一 host:port，IPv6 使用方括号。不同实例配置同一地址时使用不同静态标签。
统一输出管线的过滤、重命名和 relabel 规则仍然适用。

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
同实例同一规范化主机的不同端口共享探测，仍按完整 address 分别输出。
首轮分散在 [0, interval)，后续按完成时间加 interval 和 ±10% 抖动调度，同主机不重入。
每轮重新解析 DNS，优先 IPv4、其次 IPv6，各地址族内稳定排序选择一个地址；
不表示全部解析地址均在线，也不跨 DNS 别名或实例去重。支持带 zone 的 IPv6 字面量。
任一类健康指标启用时校验身份标签和端点；两类都关闭可沿用旧端点和静态标签配置。

raw ICMP 需要运行账户具有相应权限，如 Linux CAP_NET_RAW 或 Windows 相应权限；
`icmp_privileged=false` 请求非特权 ICMP socket，支持程度依赖系统。
运行权限或探测错误仅影响 ICMP 指标，不中止 gNMI。真实环回测试需设置
`GNMI_TEST_ICMP=1` 后运行 `go test ./inputs/gnmi -run TestICMPNativeLoopback -v -count=1`。
普通单测无需 ICMP 权限。上线前另行验证设备 ACL、丢包、IPv6 和部署账户权限。

实现说明：gNMI 使用仓库已有的 `golang.org/x/net/icmp`，直接控制 socket 读写截止时间
与取消关闭；不使用锁定版本 pro-bing 的无界发送重试。任一次发送错误都会使整轮无效。
`icmp_count` 上限为 65535，避免 Echo 序号溢出。
