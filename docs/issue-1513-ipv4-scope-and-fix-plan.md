# Issue 1513 IPv4 开关影响范围和修复方案

- 创建日期：2026-11-05
- 更新日期：2026-11-11
- 状态：候选实现，已修复三项 P2 评审问题，待合并
- 代码基线：`f2d1875b`，以下“基线行为”指该提交；工作区已有未提交候选试改，不代表方案已完成或可合并
- 关联反馈：[Issue 1513](https://github.com/flashcatcloud/categraf/issues/1513)、[Issue 1506](https://github.com/flashcatcloud/categraf/issues/1506)

`PREFER_IPV4_STACK=true` 在基线中只约束 writer 和 heartbeat 的普通 TCP 拨号。出口 IP 探测另行使用 UDP 域名拨号，仍会产生 AAAA 查询。仓库中还存在其他独立拨号、预解析和第三方客户端，需要先明确开关要覆盖哪些通信，再逐条接入和验收。

本轮实现覆盖自身 IP 探测、Agent 服务通信、日志链路和公共网络组件。采集插件的独立客户端、数据库 SDK 与内置 Prometheus 单列为待决范围，不能将前述修复描述成“整个进程不再产生 AAAA 查询”。若最终目标是统一整个进程的普通连接，这些待决路径也必须纳入实施。

## 问题根因和持续查询来源

[config/config.go](../config/config.go) 的 `GetOutboundIP()` 从 writer URL 中选择探测地址，随后调用 `net.Dial("udp", addr)`，利用连接的本地地址确定出口 IP。UDP 探测本身不需要发送业务数据，但域名地址仍需要解析；纯 Go 解析路径中的普通 `udp` 会同时查询 A 和 AAAA。

[config/hostname.go](../config/hostname.go) 在未设置 `HOSTIP` 时执行以下调用：

1. `InitHostInfo()` 在启动时调用一次 `GetOutboundIP()`。
2. `HostInfoCache.update()` 每分钟重新调用一次。

`GetHostIP()` 平时读取缓存，不是每次读取都发起 DNS 查询。周期性更新独立于 writer 的 HTTP 连接池，因此 writer 一直复用连接也无法消除这部分查询。多个实例的定时探测会累积成持续的集群 DNS 流量；搜索域和重试还可能进一步放大查询数。

现有公共封装只改写 `tcp`，出口 IP 探测既未接入该封装，也不属于 TCP 路径。修复需要覆盖 UDP；只把调用换成原封装仍不足以解决问题。

## 扫描方法和结论边界

扫描覆盖仓库自有 Go 源码中的直接拨号、`net.Dialer`、HTTP Client 和 Transport 创建、`Resolve*Addr`、`LookupIPAddr`，以及公共组件的调用方。第三方客户端按其创建入口和可配置的拨号、解析、Transport 接口继续追踪。

下表区分三种结论，避免把静态扫描结果当成全部实测结果：

- **已复现**：通过实际调用和本地 DNS 服务观察到查询类型。
- **代码确认**：源码存在独立的双栈拨号或解析入口，是否实际查询 DNS 取决于域名、配置和运行环境。
- **依赖待验**：已发现独立 SDK 入口，需结合所用版本验证其内部解析、代理和认证路径。

IP 字面量、hosts 命中和缓存可能不产生 DNS 请求。以下风险均以需要解析域名的运行条件为前提，不表示每条路径每次调用都会查询 AAAA。

## 影响范围和建议边界

### 自身 IP 探测和 Agent 服务通信

| 路径 | 基线入口 | 扫描结论 | 建议处理 |
| --- | --- | --- | --- |
| 出口 IP 探测 | `config/config.go`、`config/hostname.go` | 已复现；普通 UDP，启动及每分钟执行 | 必须纳入，开启时使用 `udp4` |
| 指标 writer | `writer/writer.go` | 已接入 TCP 封装 | 保留并纳入回归测试 |
| 心跳 | `heartbeat/heartbeat.go` | 已接入 TCP 封装 | 保留；同时检查其读取的 host IP |
| 远程配置 | `inputs/http_provider.go` | 代码确认；自行创建 HTTP Transport | 接入 HTTP 策略，保留 TLS、认证、请求头和查询参数 |
| Ibex RPC | `ibex/client/cli.go` | 代码确认；直接 `net.DialTimeout("tcp", ...)` | 接入拨号策略，保留超时和选服逻辑 |
| 升级下载 | `agent/update/update_linux.go`、`update_windows.go` | 代码确认；直接 `http.Get` | 使用遵循策略的私有客户端，保留代理、TLS 和重定向行为 |

这些路径属于 Agent 自身运行所需通信，建议统一受同一开关控制。HTTP 服务监听、Unix socket 和本地接口枚举不属于域名拨号修复对象。

### 日志链路

| 路径 | 基线入口 | 扫描结论 | 建议处理 |
| --- | --- | --- | --- |
| HTTP 日志上报 | `logs/client/http/destination.go` → `pkg/httpx/transport.go` | 代码确认；公共 Transport 使用独立 Dialer | 在公共组件接入，覆盖重连和客户端重建 |
| TCP 日志上报 | `logs/client/tcp/connection_manager.go` | 代码确认；独立 TCP Dialer | 接入策略，保留连接超时、TLS 和重试 |
| SOCKS 日志代理 | 同上 | 代码确认；代理的 forward dialer 独立 | 约束到代理的本地连接，保留 SOCKS 认证和目标域名传递 |
| Kafka 日志发送 | `logs/client/kafka/destination.go` | 依赖待验；Sarama 自行建立连接 | 核对 Dialer 接口，保留超时、KeepAlive、本地地址、TLS 和 SASL |
| Docker 日志采集 | `logs/util/docker/docker.go` | 依赖待验；Docker SDK 自行创建 Transport | 约束远程 TCP daemon，验证 TLS、API 协商和代理；保留 Unix socket、named pipe |
| Kubelet 地址发现 | `logs/util/kubernetes/kubelet/kubelet_hosts.go` | 代码确认；直接调用 `LookupIPAddr` | 开启时在解析入口请求 IPv4，不先查双栈再过滤 |
| Kubelet HTTP 请求 | `logs/util/kubernetes/kubelet/kubelet_client.go` | 代码确认；克隆默认 Transport | 在完成 TLS 配置后应用策略，保留证书、token 和超时 |

Kubelet 的 `LookupAddr` 是反向 PTR 查询，不是 AAAA 查询，不应为了本问题移除。

### 公共组件及其插件调用方

修改公共组件会直接扩大部分插件的行为范围，必须在本次范围中明确说明。

| 组件 | 已确认调用方 | 接入后影响 |
| --- | --- | --- |
| `pkg/netx` | writer、heartbeat；拟扩展的自身通信 | 普通 TCP/UDP 使用 IPv4；需要单独提供 IPv4 地址查询入口 |
| `pkg/httpx.CreateHTTPTransport` | HTTP 日志上报 | 日志连接和重建后的客户端遵循开关 |
| `pkg/httpx.CreateHTTPClient`、`NetDialer` | `inputs/http_response`、`inputs/nsq`、`inputs/json_exporter` | 这些插件通过公共 HTTP 组件建立的连接也受开关约束 |
| `pkg/proxy.TCPProxy` | `inputs/x509_cert` 的相关 TCP 路径 | 直连或到代理的本地连接受约束；不等于该插件所有协议都已覆盖 |
| `pkg/proxy.Socks5ProxyConfig` | 公共 SOCKS 工具入口 | forward dialer 可统一策略；实际运行影响以调用方为准 |

建议允许上述明确、可列举的公共组件影响，并为调用方补充说明。具有显式 IPv6 监控需求的实例需要按最终选定的优先级规则验证。

### 插件独立网络逻辑和内置 Prometheus

以下路径尚需决定是否进入本次实施范围。即使完成前面各组，它们也不会自动全部遵循开关。

| 类别 | 已发现入口 | 问题或实施要求 |
| --- | --- | --- |
| gNMI ICMP | `inputs/gnmi/icmp.go` | `LookupIPAddr` 后优先选择 IPv4；选择发生在 DNS 查询之后 |
| SNMP ping | `inputs/snmp/instances.go` | `ResolveIPAddr("ip", host)` 使用独立解析路径 |
| ZooKeeper | `inputs/zookeeper/zookeeper.go` | 先 `ResolveTCPAddr("tcp", host)`，再进行明文或 TLS 拨号 |
| TCP/UDP 探测 | `inputs/net_response/net_response.go` | TCP 直接拨号；UDP 单独解析后使用 `DialUDP` |
| 证书采集 | `inputs/x509_cert/x509_cert.go` | 同时有公共代理路径和独立 UDP、TCP、SMTP 等路径，不能只改其中一处 |
| 独立 HTTP 客户端 | 如 `inputs/cloudwatch`、`haproxy`、`jolokia`、`nginx`、`prometheus`、`kubernetes` 等 | 自建 Transport 或 SDK 客户端，不全部经过 `httpx` |
| 数据库和中间件 SDK | MySQL、PostgreSQL、MongoDB、Redis、SQL Server、Kafka 等 | 核对各 SDK 的预解析、拓扑发现、拨号回调和 TLS；不能只改外层客户端 |
| gRPC 和云 SDK | `inputs/gnmi/handler.go`、`inputs/googlecloud/instances.go` 等 | 名称解析可能先于 Dialer，须验证 resolver、负载均衡、认证及代理路径 |
| 内置 Prometheus | `prometheus/prometheus.go` | 抓取、服务发现、通知和远程写入分别创建组件，需分别检查 HTTP 和 DNS 注入接口 |

`inputs/prometheus` 插件和仓库顶层的 `prometheus` 子系统是两个入口，应分别验收。SDK 创建入口的发现不等于其所有内部路径已经实测或完成适配。

若纳入这一组，实施前必须将各调用方细化为接入清单，逐项标记“已适配”“明确例外”“未完成”。存在未完成项时，不应发布整个进程统一 IPv4 的保证。

## 开关语义和兼容性

建议继续复用 `PREFER_IPV4_STACK`，不新增同义开关。仅精确字符串 `true` 启用；未设置、空字符串、`false`、`TRUE`、`1` 和带空格的值保持基线策略。

| 场景 | 建议规则 |
| --- | --- |
| 受控的普通 `tcp`、`udp` | 开启时分别使用 `tcp4`、`udp4` |
| 受控的域名地址查询 | 直接请求 IPv4 地址，如 `LookupIP(ctx, "ip4", host)` |
| IPv4 解析或连接失败 | 不回退 IPv6；保留调用方已有错误处理和本地接口回退逻辑 |
| 明确指定 `tcp4`、`udp4`、`tcp6`、`udp6` | 公共封装保留显式网络类型 |
| 受控普通 TCP/UDP 的 IPv6 字面量目标 | 与现有 writer 开关行为一致，会因地址族不匹配失败 |
| 插件明确配置的 IPv6 专项探测 | 建议保留配置语义；纳入插件改造时明确优先级并单测 |
| DNS 监控显式查询 AAAA | 保留查询类型，这本身就是监控目标 |
| DNS 请求使用 IPv6 传输 | 与 AAAA 查询类型分开处理；本方案不修改 DNS 服务器地址或系统 DNS 传输策略 |
| Unix socket、named pipe、监听端 | 保留原行为 |

客户端在创建时捕获策略，重连沿用该策略；一次性的出口 IP 探测在调用时读取。运行中修改环境变量不作为受支持的切换方式，部署时通过重启或滚动更新生效。

出口 IP 从 IPv6 变为 IPv4 时，还可能改变心跳 `host_ip`、`$ip` 展开、引用该值的 hostname 和标签。上线应核对主机标识和时序连续性，不能只检查 DNS QPS。

代理场景约束的是到代理的本地连接。代理端解析目标域名的查询类型由代理决定；IPv6-only 代理可能因启用限制而不可达。任意自定义 SDK proxy dialer 必须单独适配，不能通过覆盖用户代理配置来获得 IPv4 行为。

## 修复设计

### 公共拨号和解析策略

在 `pkg/netx` 集中管理环境变量读取、网络类型转换、Dialer 包装和地址查询，避免各调用方复制判断代码。

拨号封装保留调用方已配置的 `net.Dialer`，继续由标准库处理地址轮换、context、取消、超时、KeepAlive 和本地地址。扩大现有 `DialContextFromEnv` 对普通 UDP 的支持，并为一次性拨号或 SDK 接口提供适配方式。

解析封装在启用时直接请求 `ip4`，关闭时保留原有解析行为。对于 `ResolveTCPAddr`、`ResolveUDPAddr` 等预解析，必须在解析之前选择地址族。先查询双栈、再过滤 IPv6 结果，无法降低 AAAA 查询量。

不手工选择第一个 A 记录来替代标准库连接逻辑，不重写 URL、Host 或 TLS ServerName；不修改 `net.DefaultResolver`、`http.DefaultTransport`、系统 hosts 或 `resolv.conf` 作为产品实现。

### HTTP 和代理适配

HTTP 组件在代理、TLS 和自定义拨号参数配置完成后应用策略。共享 Transport 应克隆后使用，不能修改默认全局实例。需要验证克隆和增加 DialContext 不改变 HTTP/2 协商、连接复用、重定向和证书校验行为。

存在自定义 `Dial`、`DialContext`、`DialTLS` 或 `DialTLSContext` 时，保留其实际拨号能力和设置。对于自定义 RoundTripper，应追踪其底层连接入口，不把外层包装视为已经覆盖。

HTTP CONNECT 和 SOCKS 代理需要约束 forward dialer。目标域名仍按原协议传递给代理，不在本地提前解析。SDK 的认证、重试和独立 token 客户端也应列入相应依赖的验证项。

### 出口 IP 探测接入

`GetOutboundIP()` 的普通 UDP 拨号使用统一策略。保持 writer 地址选择、端口补全、无 writer 时的默认探测地址和 `getLocalIP()` 回退逻辑。

保留 `HOSTIP` 优先级：设置后继续跳过启动和定时出口 IP 探测。临时缓解方式是向每个实例设置正确的自身 IP，不应给所有 Pod 配置同一个值。更换 writer 为字面量 IP 涉及 TLS 和地址维护，不作为通用规避方案。

## 验证计划和已有证据

### 已有隔离测试

本地基线复现通过实际 `GetOutboundIP()` 调用和本地 DNS 服务统计查询。纯 Go 解析器下，连续调用 5 次得到：

| 实现和设置 | A 查询 | AAAA 查询 |
| --- | ---: | ---: |
| 基线普通 UDP，开关关闭 | 5 | 5 |
| 基线普通 UDP，开关开启 | 5 | 5 |
| IPv4 UDP 对照路径 | 5 | 0 |

2026-11-05 在 `10.129.0.77` 的 Linux amd64 环境运行了候选修复的测试二进制。本地工具链为 Go 1.26.8，交叉构建使用 `CGO_ENABLED=0`；服务器默认 Go 1.18.1 未参与构建。测试使用进程内纯 Go resolver 和临时端口的本地 DNS 服务，不修改服务器 DNS 配置。

候选出口 IP 测试验证了连续 5 次探测在开启后得到 A=5、AAAA=0。公共组件测试覆盖 HTTP/HTTPS、连接复用、强制断连、HTTP 代理、IPv6-only 目标、证书校验、显式网络保留、环境变量捕获、取消和超时，均通过。远端临时测试目录和二进制已清理。

这些结果证明基础策略可行，不等于日志 SDK、全部插件、cgo 系统解析器或真实 K8s 集群已通过验收。本轮修复的后续回归和构建结果见下文验证记录。

### 实施后的验收要求

| 验证层 | 必须观察的结果 |
| --- | --- |
| 开关关闭 | 与基线策略一致；不强制 IPv4，不改变显式网络类型 |
| 自身 IP 探测 | 启动及多轮更新不产生目标域名 AAAA；`HOSTIP` 能绕过探测；失败时保持原回退行为 |
| 实际调用方 | 每个纳入的客户端都测试域名连接；不能仅测试公共封装 |
| 连接和 TLS | 复用及断连重建均正确；Host、SNI、可信与不可信证书行为符合原配置 |
| 代理 | 到代理的本地解析受控，认证及目标域名传递正确；不把代理端 DNS 算入本地保证 |
| SDK | 预解析、拓扑发现、认证客户端、重试和自定义代理分别核对 |
| Linux 解析器 | 使用发布对应的 Go 版本和构建参数；分别覆盖纯 Go，以及支持的 cgo/libc 路径 |
| 跨平台 | Linux amd64/arm64 和 Windows 构建；验证 Unix socket 和 Windows named pipe |
| K8s | 覆盖真实搜索域、ndots、Pod 启动和周期刷新；同时观察 AAAA/A QPS、上报成功率和主机标识 |

远端系统解析器验证如需独立 `resolv.conf`，应使用隔离的网络和挂载命名空间。直接修改测试主机 DNS 配置不属于验证步骤。

## 当前实施范围和后续扩展

1. **本轮实施范围**：自身 IP 探测、Agent 服务通信、日志链路及公共组件，同时接受上文列出的公共组件对插件调用方的明确影响。
2. **扩展范围**：是否将插件独立 HTTP/TCP/UDP、数据库与云 SDK、gRPC、内置 Prometheus 全部纳入同一开关。
3. **插件优先级**：扩展范围若获采用，确认“插件显式 IPv6 配置优先，普通连接受全局开关约束”的规则。

本轮已同步 `conf/config.toml` 和 `k8s/README.md` 的覆盖范围、IPv6 限制及标识变化说明。独立插件和内置 Prometheus 的扩展仍需单独确定范围，不属于本轮完成项。


## 三项 P2 问题的修复和回归

### 共享 Transport 的 HTTP2 行为

旧实现通过 `TLSClientConfig` 等字段猜测隐式 HTTP/2 支持，但 `Transport.Clone()` 会初始化源对象的协议设置。同一源对象第二次被包装时，猜测结果可能发生变化，导致 ALPN 协商为 h2、实际发送 HTTP/1.1，请求报 malformed HTTP response。

当前实现先调用 `Clone()` 完成同步初始化，再根据源 Transport 已初始化的 h2 handler 保留 HTTP/2 支持。显式 `Protocols`、禁用 HTTP/2 的 `TLSNextProto` 空映射、自定义 TLS 和拨号配置继续遵循原协议设置。

回归测试使用本地支持 HTTP/2 的 HTTPS 服务和可信测试证书，覆盖同一源对象连续包装、已用于 HTTP 请求后再包装、多个并发包装客户端，以及实际 HTTP/1-only 请求。测试检查响应协议和请求成功，不仅检查配置字段。

### Kafka canonical 解析的 IPv4 策略

当 `PREFER_IPV4_STACK=true` 且 `Net.ResolveCanonicalBootstrapServers=true` 时，日志客户端在创建 Sarama producer 前执行以下步骤：

1. 对每个 bootstrap 域名仅查询 IPv4 地址。
2. 对得到的 IPv4 地址进行 PTR 反向查询，保留 canonical hostname 和端口，用于 TLS/GSSAPI；多个地址仍逐个处理，按主机名和端口去重。
3. 复制 Sarama 配置，在副本中关闭已完成的 canonical 预解析，并接入 IPv4 Dialer。共享配置保持原样，每个目标或重建客户端都重新处理自己的 bootstrap 列表。

解析受 `Net.DialTimeout` 约束。没有 A 记录、缺少 PTR、地址格式不合法或 context 取消时返回错误，不回退双栈，也不静默改用原始域名。

同时启用 IPv4 开关、canonical 解析和自定义代理 Dialer 时，初始化明确返回不支持该组合的错误。代理可能负责远端 DNS，不能直接替换为本地 canonical 查询。关闭 IPv4 开关时仍使用 Sarama 原有配置和解析流程；仅启用自定义代理而未启用 canonical 时保留代理自己的策略。

集成测试通过本地 DNS fixture 和真实 Sarama mock broker 验证客户端创建、消息发送、重建和 DNS 查询计数，确保 AAAA=0 且 PTR 路径实际执行。另有回归覆盖配置副本、TLS/SASL/TCP 设置保留、重复地址去重、代理组合错误、缺少 A/PTR 和取消行为。

### 用户可见的范围说明

`conf/config.toml` 和 `k8s/README.md` 已列出 Agent 自身、日志链路及公共组件调用方，删除“其他采集客户端不受影响”的旧承诺。说明中同时列出未覆盖的独立客户端和内置 Prometheus、自定义 Kafka 代理边界、IPv6-only 目标失败，以及 `host_ip`、`$ip`、hostname 和标签可能变化的影响。

### 本轮验证记录

2026-11-11 完成以下检查：

- 本地 Go 1.26.8 下，`netx`、`httpx`、`proxy`、config、writer、heartbeat、远程配置、Ibex、日志客户端、Docker/Kubelet 工具，以及公共组件的 http_response、NSQ、JSON exporter、x509_cert 调用方通过对应的测试或编译检查；有测试的包使用 `-race -count=1`。受影响实现通过 `go vet`。
- 发布基线 Go 1.25.10 下，`pkg/netx` 和 `logs/client/kafka` 的 race 回归连续运行三轮通过。精确 DNS 计数用例将每主机连接数限制为 1，以隔离标准库推测性拨号对连接数量断言的影响。
- 使用 Go build overlay 临时还原旧 HTTP/2 判断，新共享 Transport 测试失败；使用修复后的实现，同一用例通过。此验证未覆盖或回写工作区实现文件。
- 完整主程序以 `CGO_ENABLED=0` 分别构建 Linux amd64、Linux arm64 和 Windows amd64，全部通过。
- `10.129.0.77` 上的 Linux amd64 测试二进制通过公共网络组件及 Kafka 回归，包含实际 HTTPS HTTP/2 请求和 Kafka producer 创建、消息发送、重建、A/PTR 解析及 AAAA=0 断言。该轮使用本地 Go 1.26.8 交叉构建的无 cgo 二进制及纯 Go DNS fixture，不属于 libc 系统解析器或生产 Kafka TLS/GSSAPI 集群验收。
- `git diff --check` 和文档本地链接检查通过。远端测试使用临时目录与临时 DNS 端口，服务器 DNS 配置保持原样。

本轮整体复查未发现新的阻塞项。真实 K8s 集群规模验证、cgo/libc 系统 DNS 验收及独立插件扩展，继续按前述边界单独进行。
