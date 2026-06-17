# gNMI (gRPC Network Management Interface) 采集插件

该插件基于 [gNMI](https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md) 协议的 Subscribe 方法，订阅并采集网络设备的遥测 (Telemetry) 数据。
插件支持 TLS 认证和加密，与设备供应商无关，支持任何兼容 gNMI 规范的平台。

该插件 fork 自 `telegraf/inputs.gnmi`。对于 Cisco 设备，它特别针对 Cisco IOS XR (64-bit) 6.5.1, Cisco NX-OS 9.3 以及 Cisco IOS XE 16.12 及以上版本产生的遥测数据进行了优化。

## 配置说明

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
