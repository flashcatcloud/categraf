# Issue 1513 插件域名解析 IPv4 策略修复方案

- 日期：2026-11-11
- 状态：第一至第四阶段代码已接入；逐路径验证和明确例外见[累计实施记录](issue-1513-plugin-ipv4-implementation.md)
- 基线：主仓库 `main` 的 `022041e8`，已包含基础 IPv4 策略及旧 HTTP Dial 回调的 CI 兼容修复
- 前置方案：[IPv4 开关影响范围和基础修复](issue-1513-ipv4-scope-and-fix-plan.md)
- 关联反馈：[Issue 1513](https://github.com/flashcatcloud/categraf/issues/1513)

本方案处理剩余两类客户端：插件自行创建的 HTTP 客户端，以及数据库、Kafka、gRPC、云服务等独立 SDK。目标是让纳入清单的普通域名连接在 `PREFER_IPV4_STACK=true` 时，直接按 IPv4 地址族解析和建连，避免“先查询 AAAA，再丢弃 IPv6 地址”。

基础修复已覆盖 Agent 自身、日志链路及 `pkg/httpx`、`pkg/proxy` 的部分调用方。它不会自动覆盖本方案列出的独立客户端。后续应按清单逐项交付；任何依赖接口阻塞项未解决前，发布说明都必须保留该项的未覆盖状态。

## 目标和兼容规则

1. 仅精确字符串 `PREFER_IPV4_STACK=true` 启用。关闭时保留当前客户端创建、解析、重试和协议行为，不强制切换 Go/cgo resolver。
2. 普通 `tcp`、`udp` 和普通 IP 地址查询使用相应 IPv4 策略。显式 `tcp6`、`udp6`、独立 IPv6 探测和 DNS 监控主动查询 AAAA 保留配置语义。
3. 地址族必须在首次地址解析前生效。解析结果过滤、业务地址改写为第一个 IP、只设置最终连接的 Dialer，均不能代替完整适配。
4. 保留 URL、Host、TLS ServerName、证书校验、认证、超时、KeepAlive、本地地址绑定、重定向、连接池和 SDK 重试策略。
5. IPv4-only 模式下，受控普通连接访问 IPv6-only 目标应失败，不回退双栈。Unix socket、named pipe、串口和监听端保留原行为。
6. HTTP/SOCKS 代理约束的是可控制的本地 first hop。代理端解析不属于本地查询保证；任意用户自定义 Dialer/Resolver 不得被静默覆盖。
7. 每个实例或客户端创建时确定策略。重连、池扩容、认证刷新、发现新节点和客户端重建必须沿用策略；运行中修改环境变量不作为支持的动态开关。
8. 保留 DNS SRV、TXT、PTR 等协议必需查询。“受控路径无 AAAA”不等于“无 DNS 查询”，也不要求 DNS 请求一定通过 IPv4 发送。

## 现有能力和新增适配边界

复用 `pkg/netx` 的 `NewDialer`、`DialContextFromEnv`、`NetworkFromEnv`、`LookupIPAddr`、`HTTPTransportFromEnv` 和 `HTTPClientFromEnv`。公共 HTTP 封装已处理 HTTP/2 初始化、共享 Transport 克隆、Context 回调优先级及旧 Dial/DialTLS 兼容。

`HTTPClientFromEnv` 只识别标准 `*http.Transport`。API Key、签名、OAuth、追踪等自定义 RoundTripper 必须先定位底层 Transport，在装饰之前应用策略。SDK 专用类型不应引入 `pkg/netx`；可在插件内部或独立 SDK 适配包中封装。

需要增加的能力以调用方需求为准：捕获策略的 `LookupHost`/`LookupIP` 适配、已有 DialFunc 的网络类型包装、客户端工厂和协议测试 fixture。禁止通过修改 `net.DefaultResolver`、`http.DefaultTransport`、全局 gRPC resolver 注册表或进程 DNS 配置来替代逐客户端接入。

## 自建 HTTP 客户端接入清单

以下为基线源码确认的入口。表中“接入”是待实施动作，不表示已经验证完成。

| 插件 | 主要源码入口 | 接入动作与专门关注项 |
| --- | --- | --- |
| nginx | `inputs/nginx/nginx.go` 的 `createHTTPClient` | 完成 TLS、重定向配置后应用策略；保留当前禁用 keep-alive 的设置 |
| nginx_upstream_check | `inputs/nginx_upstream_check/nginx_upstream_check.go` | 包装现有 Dialer，保留接口绑定的 LocalAddr 和超时 |
| apache | `inputs/apache/exporter/collector.go` | 在 exporter 创建 HTTP 客户端时接入，保留认证及证书配置 |
| haproxy | `inputs/haproxy/exporter.go` 的 `fetchHTTP` | 包装值类型 HTTP Client 的 Transport；HTTP 路径和原有代理开关均需测试 |
| rabbitmq | `inputs/rabbitmq/rabbitmq.go` | 在 TLS、代理、认证配置完成后接入 |
| tomcat | `inputs/tomcat/tomcat.go` | 保留 TLS 和客户端总超时 |
| logstash | `inputs/logstash/logstash.go` | 保留证书、认证和超时 |
| influxdb | `inputs/influxdb/influxdb.go` | 保留现有 HTTP 超时和 Transport 设置 |
| clickhouse | `inputs/clickhouse/clickhouse.go` | 实例 HTTPClient 初始化时接入，避免每次采集新建连接池 |
| elasticsearch | `inputs/elasticsearch/elasticsearch.go` | 先包装底层 Transport，再叠加 API Key/AWS 签名；逐个覆盖 TLS 与非 TLS 分支 |
| jolokia | `inputs/jolokia/client.go` | 公共 Jolokia 客户端接入，回归 `jolokia_agent`、`jolokia_proxy` 两个调用方 |
| jenkins | `inputs/jenkins/jenkins.go` | 保留 Basic Auth、超时及请求头 |
| cloudwatch | `inputs/cloudwatch/cloudwatch.go` | 业务 HTTPClient 保留 30 秒 Dial/KeepAlive、代理和重试；凭据链另按云 SDK 方案处理 |
| redfish | `inputs/redfish/redfish.go` | 为每个地址创建的客户端分别接入，保留认证、TLS 和会话行为 |
| tengine | `inputs/tengine/tengine.go` | 保留现有超时和 TLS |
| nats | `inputs/nats/nats.go` | 此入口是监控 HTTP API；保留 TLS 与认证，不能据此声称所有 NATS 协议都已适配 |
| xskyapi | `inputs/xskyapi/xskyapi.go` | 包装现有 Dialer，保留 TLS、代理和超时 |
| appdynamics | `inputs/appdynamics/instances.go` | 包装现有 Dialer，保留本地地址绑定、TLS 和认证 |
| Docker 指标插件 | `inputs/docker/client.go` 的 `NewClient`、`NewEnvClient` | SDK `WithHost` 会重新配置 Transport；必须在最终 host/TLS 配置后接入，验证环境变量和显式 endpoint 两个入口 |
| cadvisor | `inputs/cadvisor/instances.go` | 保留 Kubelet/Pod API 的 TLS、认证和地址构造 |
| Kubernetes | `inputs/kubernetes/kubernetes.go` | 在 RoundTripper 创建和更新路径接入，保留 token、CA 和请求超时 |
| Prometheus 插件 | `inputs/prometheus/prometheus.go`、`consul.go` | 普通抓取 HTTP 客户端与 Consul 发现客户端分别接入；与顶层内置 Prometheus 子系统分开验收 |
| consul | `inputs/consul/consul.go` | 保留 SDK config 的 TLS、token、代理和连接池 |
| emc_unity | `inputs/emc_unity/emc_unity.go` | 所有地址的实例客户端分别接入 |
| phpfpm | `inputs/phpfpm/phpfpm.go` | 本组处理 HTTP 客户端；FastCGI TCP/Unix 路径另列旁路清单 |
| traffic_server | `inputs/traffic_server/traffic_server.go` | 为 HTTP 统计请求接入策略 |
| bind | `inputs/bind/bind.go` | 处理值类型 HTTP Client，保留统计接口行为；不修改主动 DNS 监控查询类型 |
| rocketmq_offset | `inputs/rocketmq_offset/rocketmq.go` | 替换登录和业务请求对 `http.DefaultClient` 的直接使用，改为实例私有、可复用的策略客户端 |
| hadoop | `inputs/hadoop/hadoop_collector.go` | 普通 HTTP 和 SPNEGO HTTP 客户端分别处理；KDC/认证网络另行核对 |
| huatuo | `inputs/huatuo/huatuo.go` | 先确认目标是否固定为 IP/本地地址；域名可达路径接入，固定字面量路径记录无需修改的依据 |
| x509_cert | `inputs/x509_cert/x509_cert.go` 的 `createHTTPClient` | 补齐独立 HTTP 请求路径；现有公共 TCP 代理覆盖不代表 SMTP/UDP 等协议已覆盖 |

XML-RPC 的 `inputs/supervisor` 和 `inputs/node_exporter/collector/supervisord.go` 也要检查 SDK HTTP 客户端：普通 HTTP 走统一策略；Unix socket 的专用 DialContext 保留原网络和路径。

### 标准改造顺序

1. 按当前配置完整创建 Dialer、TLS、proxy、超时、连接池与重定向策略。
2. 在底层 `*http.Transport` 上应用 `netx.HTTPTransportFromEnv`，或在已完成配置的标准 HTTP Client 上应用 `HTTPClientFromEnv`。
3. 再添加 API Key、签名、OAuth 或追踪等装饰器，将最终客户端保存到实例中。
4. SDK 仍会重配 Transport 的场景，在 SDK 最后一个配置阶段处理；不得在 `WithHost` 等操作前设置后被覆盖。
5. 为重新初始化、错误后重建和配置更新路径复用同一个客户端工厂；替换旧客户端时按原生命周期关闭空闲连接。

不要求把所有客户端机械改写成 `httpx.CreateHTTPClient`。已有插件的代理默认值、HTTP/2、TLS 或 keep-alive 设置可能不同，迁移应保留这些差异。

### HTTP 专项陷阱

- Elasticsearch 的鉴权/签名包装属于自定义 RoundTripper，仅包装最终 Client 可能完全绕过底层策略。适配所有构造分支，避免重设 Transport 时丢失装饰器。
- CloudWatch 即使业务 API 无 AAAA，STS 或凭据刷新仍可能查询。业务客户端与凭据客户端必须合并验收。
- Docker 指标客户端与已改造的 Docker 日志客户端是两个实现。建议提取可共享的 Docker 选项适配，不能复制后漏掉 `NewEnvClient`、named pipe 或 API 协商。
- 具有本地接口绑定的插件，在普通 TCP 被限定到 IPv4 后，IPv6 本地地址可能不兼容。应明确返回错误并记录部署影响，不能悄悄换接口。

## 独立 SDK 的逐项设计

本节接口按当前依赖源码核对。接口存在只说明可尝试接入；完成标准是实际协议、发现、认证、重连的 DNS 与兼容测试通过。

| SDK 和版本 | 当前入口 | 可用接口或阻塞点 | 建议方案 |
| --- | --- | --- | --- |
| MySQL v1.6.0 | `inputs/mysql/mysql.go` 的 `sql.Open` | `mysql.ParseDSN`、`NewConnector`、`Config.Net`；该版本不能假设存在每配置 DialFunc | 解析并完成默认值处理后，将普通 `tcp` 转为 `tcp4`，用 Connector/`sql.OpenDB` 建池；保留 Unix、显式族和自定义注册网络 |
| pgx v5.9.2 | `inputs/postgresql/connection.go`、`postgresql.go` | `ConnConfig.LookupFunc` 在 `DialFunc` 之前执行 | 同时注入仅 IPv4 的 LookupFunc 和保留原设置的 DialFunc 包装；覆盖多 host、fallback、取消连接 |
| go-redis v8.11.5 | `inputs/redis`、`inputs/redis_sentinel` | `Options.Network`、`Dialer`；默认 Dialer 同时执行 TLS 握手 | 优先设置 Network，沿用 SDK 的 TLS-aware 默认 Dialer；需要自定义回调时完整保留握手、超时和 KeepAlive |
| MongoDB v1.17.7 | `inputs/mongodb/exporter/exporter.go` | `SetDialer` 控制数据库连接；`SetHTTPClient` 控制 OCSP 和部分认证 HTTP 请求 | 同时注入 IPv4 ContextDialer 和私有策略 HTTPClient；保留 SRV/TXT、目标域名与 TLS/认证行为，覆盖节点发现、OCSP、认证刷新和重建 |
| SQL Server v0.12.2 | `inputs/sqlserver/sqlserver.go` | Connector 有 Dialer，但 `tds.go` 的 `dialConnection` 先直接 `net.LookupIP(p.Host)` | 属于依赖接口阻塞项，不能只换 Connector；先解决解析 hook，再适配 TCP、UDP Browser 和路由重连 |
| Sarama v1.48.0 | `inputs/kafka/exporter/exporter.go` 两处 `NewClient` | `Net.Proxy.Dialer`；canonical bootstrap 会独立预解析 | 复用已验证的日志 Kafka 策略，但提取为不依赖日志模块的适配包；每次创建/重建都处理配置副本和 bootstrap 列表 |
| gRPC v1.83.2 | `inputs/gnmi/handler.go` | `WithContextDialer` 会跳过默认代理流程；`WithResolvers` 不覆盖代理地址使用的全局 DNS builder | 保留原生 delegating resolver/CONNECT；补齐逐 ClientConn 的底层拨号和代理 DNS 注入能力，再应用 IPv4 策略；当前接口缺口作为依赖阻塞项 |
| Google Monitoring v1.24.3 | `inputs/googlecloud/instances.go` | gRPC 客户端选项与凭据 HTTP 链路相互独立 | 将 gRPC resolver/dialer 与 ADC/OAuth/STS HTTP 策略一并接入，验证连接池和 token 刷新 |
| AWS SDK v2 | `inputs/cloudwatch`、`pkg/aws/credentials.go`、Elasticsearch 的签名 RoundTripper | `LoadDefaultConfig` 的 HTTPClient 和各 Service Client 可配置；存在独立 STS 创建入口 | 在各凭据提供者创建前注入策略客户端，传给 STS/元数据与业务客户端；保留 SigV4 和重试 |
| 阿里云 CMS SDK | `inputs/aliyun/internal/manager/cms.go` 两种 CMS client | 当前 Tea 在内部创建和缓存 Transport，需核对实际公开注入接口 | 优先使用逐客户端的公开 HTTP/Transport hook；如当前版本没有 hook，按依赖阻塞项处理 |
| govmomi v0.29.0 | `inputs/vsphere/client.go` 的 `soap.NewClient` | SOAP SDK 包装 HTTP Transport，证书/root CA 在后续步骤配置 | 在首次 vim25 请求之前，完成证书/CA 后适配实际底层 Transport，保留登录会话和重新登录 |

### PostgreSQL 解析和回退

`LookupFunc` 开启时直接使用 `LookupIP(ctx, "ip4", host)` 并返回完整的 IPv4 地址列表；关闭时继续使用原回调。不能先调用原 `LookupHost` 再过滤。保持原 `DialFunc` 的 connect_timeout 等设置，在网络类型处施加策略。

覆盖主 host、Fallbacks、服务文件、Unix socket、SSL 模式、TLS 主机名和 CancelRequest。保持 DSN 原文及协议选择，不把 hostname 替换成某个 IP，也不改动已有 simple protocol/预处理语句兼容逻辑。

### MySQL 和 Redis 的低侵入接入

MySQL 应先按原 DSN 解析和归一化默认端口、地址、TLS 配置，再修改普通网络类型。`tcp4` 不是旧版本默认地址推断的 `tcp` 分支，不能先替换原始 DSN 网络字符串再依赖驱动补齐默认值。继续保留数据库连接池配置、认证、TLS ServerName 和自定义网络注册。

Redis 的 Network 只控制网络类型，默认 Dialer 仍负责明文或 TLS 建连。直接将 `Options.Dialer` 换为裸 `netx.DialContextFromEnv` 会跳过默认 TLS，这是禁止的接入方式。Sentinel 的 Unix 连接、TLS、认证和未来发现地址均需分别验证。

### MongoDB 数据库连接和内部 HTTP 双入口

当前 v1.17.7 的 `ClientOptions.SetDialer` 只覆盖数据库网络连接。驱动另有 HTTPClient：`mongo/client.go` 将它传入认证器，`topology/connection.go` 将它传入 OCSP 验证。`SetHTTPClient` 是单独的公开接口；默认值来自驱动内部的共享 HTTP Client，而不是插件的 ContextDialer。

因此接入必须在 `mongo.Connect` 创建认证器和 topology 之前完成，且后续 options 合并不能覆盖已配置的两个入口：

1. 保留原 URI 和 TLS 设置，通过 `SetDialer` 配置数据库 IPv4 ContextDialer，处理 seed、发现节点、心跳和连接池补连。
2. 读取最终 ClientOptions.HTTPClient。开启策略时，通过 `netx.HTTPClientFromEnv` 克隆其标准底层 Transport，得到实例私有的策略客户端，并通过 `SetHTTPClient` 注入；保留原 timeout、重定向、proxy、TLS 和连接复用设置。未指定客户端时，以 `options.Client().HTTPClient` 提供的 SDK 默认客户端为基准，不导入 SDK 的 internal 包，也不新增任意 HTTP 总超时。
3. 对自定义 RoundTripper 明确追踪底层 Dialer；不能克隆或注入的组件列为未支持路径，不能因 SetHTTPClient 调用成功就声称其流量已受控。关闭策略时保留 SDK 原默认/用户客户端。
4. 私有 HTTPClient 保存到插件实例，与 Mongo Client 一起重建。先完成 Disconnect，再关闭该实例拥有的 HTTP 空闲连接；构造失败时也清理。当前驱动只为其共享默认 HTTP Client 自动清理，不能依赖 Disconnect 自动关闭注入的客户端，更不能关闭用户共享客户端的连接池。

HTTP 路径至少覆盖证书上的 OCSP responder，以及使用 SDK HTTPClient 的 MONGODB-AWS 凭据获取、Azure/GCP OIDC 等认证请求。自定义 OIDC/凭据回调可能另建客户端，必须单列其出站请求和可注入能力。保留 SRV/TXT、TLS 证书验证、OCSP stapling、缓存及原有 soft-fail/revocation 语义，不通过关闭 OCSP、跳过证书验证或换认证机制减少 DNS。

验收需分别观察数据库、OCSP、认证/元数据域名的查询和实际请求。冷 OCSP 缓存下应收到有效 responder 请求且 AAAA=0；缓存命中或有效 staple 不应被强制改为联网检查。使用 Good/Revoked 响应验证原证书状态行为，无法访问 responder 时验证基线的 soft-fail 规则。认证首次获取、过期刷新、重新认证和客户端重建都要产生可观测的预期请求，不能只用一次成功的数据库连接判定覆盖完成。

### SQL Server 的依赖阻塞处理

当前驱动在 `dialConnection` 内直接调用 `net.LookupIP`，之后才调用 Connector.Dialer，因此后者无法阻止 AAAA 查询。设计选择如下：

1. 优先评估满足 Go 1.25 和现有协议兼容要求、且提供逐 Connector 解析 hook 的驱动版本；只有确认接口可用后，才能把升级作为实施方案。
2. 如无合适版本，向驱动补充最小的逐 Connector LookupIP 回调，默认行为保持原样；通过固定提交的依赖补丁交付并更新依赖/vendor，不仅修改本地 vendor 目录。
3. 开启时将解析 hook 指向 `ip4` 查询，同时让 TCP 和 SQL Browser UDP 使用公共策略。AlwaysOn 多地址并行尝试和服务端路由重连也必须沿用该 hook。
4. 保留现有 `mssql` 驱动的查询文本处理模式。当前 `NewConnector` 默认使用不同处理模式，不能机械替换 `sql.Open("mssql", ...)`；应从原有 Driver 的 `OpenConnector` 路径构造并验证 SQL 参数兼容。

在依赖修复落地前，该插件标为“未完成”。不通过解析成首个 IP 后重写 DSN 来绕开问题，这会改变多地址故障切换、TLS/SPN 和实例发现行为。

### Kafka 指标客户端和辅助发现

把日志实现中已验证的配置复制、IPv4 Dialer、canonical A/PTR 处理提取到独立包，例如 `pkg/kafkax`，由日志和指标插件共同调用。该包不得带 `!no_logs` 限制，启用 `no_logs` 构建时指标 Kafka 仍需可用。

两个 `sarama.NewClient` 调用点及重建路径统一使用工厂。TLS、SASL、canonical hostname、metadata 返回的新 broker、coordinator 和重试均要保持正确。自定义代理加 canonical 的不支持组合继续明确报错；其他自定义代理维持其明确的策略边界。

`UseZooKeeperLag` 还会创建 Kazoo/ZooKeeper 客户端，属于同一插件的辅助连接。必须继续追踪该 SDK 的 Dialer/解析接口，不能完成 Kafka broker 连接后就将整个插件标为完成。

### gRPC 原生代理和解析链路

当前 v1.83.2 的 `resolver_wrapper.go` 在设置自定义 Dialer 后跳过 delegating resolver；`internal/transport/http2_client.go` 也会直接调用该 Dialer。因此直接使用 `WithContextDialer(netx...)` 会绕过已有的 HTTPS_PROXY/CONNECT 行为。另一方面，delegating resolver 的 `proxyURIResolver` 直接调用全局 `resolver.Get("dns")`，只配置 `WithResolvers` 仍无法限制代理域名的 AAAA。两处必须一起处理。

本方案选择保留 gRPC 原生代理流程，不以裸 WithContextDialer 替换整个建连流程，也不修改全局 resolver。先评估满足当前 Go/SDK 兼容要求的依赖版本；如没有完整公开接口，向 gRPC 增加最小的逐 ClientConn 扩展点，通过固定依赖提交及相应 vendor 更新交付。以下为需要新增的能力，不是当前版本已有公开 API：

1. **底层拨号装饰点**：为默认 direct dial 和 `proxyDial` 的 TCP first hop 提供同一逐 ClientConn hook。它只包装原有 NetDialer 的网络类型，普通 TCP 使用 tcp4，保留原生 KeepAlive、context 和超时；不得复用会触发“跳过 delegating resolver”条件的现有 custom Dialer 字段。
2. **代理 DNS builder 注入点**：delegating resolver 创建代理子 resolver 时，使用 ClientConn 提供的私有 IPv4 builder，而非无条件取全局 DNS builder。目标子 resolver 和代理子 resolver 分别保留自己的更新、错误和关闭生命周期。
3. **默认路径完全保留**：不开启 IPv4 策略时不设置这两个 hook；现有代理选择、拨号和解析按基线执行。补丁不改变没有 opt-in 的其他 gRPC 客户端。

代理选择、握手和连接必须按下表明确实现与验收：

| 路径 | 实现要求 |
| --- | --- |
| 代理选择 | 沿用原生 `proxyURLForTarget` 对 `Scheme=https` 请求调用 `http.ProxyFromEnvironment` 的规则，保留 HTTPS_PROXY/https_proxy、NO_PROXY/no_proxy、localhost 和非法代理配置的处理；不另写环境变量优先级，也不擅自增加 HTTP_PROXY 回退 |
| 无代理或 NO_PROXY 命中 | 目标私有 resolver 进行 ip4 查询；底层 direct hook 使用 tcp4 建连，保留原目标 authority、负载均衡和 TLS 设置 |
| 选中代理且默认远端解析 | 保留原生 DNS target 不做本地目标解析的行为，将原 hostname:port 作为 CONNECT 目标；仅代理域名按 ip4 解析并以 tcp4 建立 first hop，不对目标做额外 A/AAAA 查询 |
| 显式 WithLocalDNSResolution | 保留本地目标解析的 opt-in 行为，目标与代理子 resolver 均使用 IPv4；沿用原生逐地址代理属性和 NO_PROXY 处理，不用“原域名永远走代理”的简化逻辑替代 |
| CONNECT 与认证 | 复用原生 `doHTTPConnectHandshake`、proxy attributes、User-Agent 和 URL Userinfo 对应的 Proxy-Authorization Basic；保留 CONNECT authority 格式及已缓冲响应字节，不把代理凭据转发为目标业务认证或写入日志 |
| 隧道之后的 TLS | TLS/gRPC 凭据仍由原 transport 执行，使用后端的 authority、SNI 和 CA 配置；不能改成代理主机名或 first-hop IP，不能跳过证书验证 |
| 失败与取消 | 代理 DNS/连接失败、CONNECT 407/非成功响应、握手超时或取消时关闭连接并返回错误，禁止自动降级直连或 IPv6；CONNECT 的 I/O 需受建连 context 控制，必要的 deadline 在成功后恢复，不泄漏取消协程 |

HTTPS_PROXY 是环境变量名，不能据此承诺新增了“到代理本身使用 TLS”的能力；代理 URL 协议支持遵循当前 gRPC 基线。新增代理协议支持应单独设计和验收。`WithNoProxy`、Unix、用户自定义 Dialer/Resolver 也需保留明确语义：尤其不能在已有 custom Dialer 之下擅自增加原先未使用的环境代理。

直连目标的私有 resolver 通过逐 ClientConn builder 进行 ip4 查询，提供完整地址集合，保留端口、authority、TLS ServerName、service config、ResolveNow、错误重试和负载均衡。不统一改写为 passthrough。DNS、passthrough、Unix、自定义 resolver 及代理模式分别测试；无法保留原语义的组合必须保持“依赖阻塞/未支持”，不能静默直连。

这项工作在两个注入点及代理回归落地前属于依赖阻塞项；只交付目标 IPv4 resolver 或一个 WithContextDialer 不算完成。若未来改为自管代理感知 Dialer，必须以独立方案完整复现上述代理选择、CONNECT、认证、取消及地址更新契约，经专项评审后再替换本方案，不作为临时捷径。

### Google Monitoring 的业务和认证连接

Google Monitoring 的 gRPC channel 使用上述保留代理能力的逐 ClientConn 策略，不直接添加裸 WithContextDialer。ADC、服务账号、外部身份、OAuth/STS、元数据服务仍可能使用独立 HTTP 客户端，应在创建 MetricClient 前确定凭据库的注入点。token 过期刷新、mTLS、代理及启用的服务发现/直通路径一并验收；不切换到 REST 模式来规避原有 gRPC 语义。

### 云 SDK 的凭据和内部客户端

AWS 应从 `pkg/aws/credentials.go` 的 root config 创建阶段注入 HTTPClient，使 AssumeRole、WebIdentity、STS 和可用的元数据凭据提供者继承策略，再配置 CloudWatch Service Client。所有使用该公共凭据组件的调用方都要计入影响范围。

`inputs/elasticsearch/pkg/roundtripper/roundtripper.go` 另外直接调用 `LoadDefaultConfig` 和 `sts.NewFromConfig`，不经过上述公共凭据组件。该入口也必须在创建和首次 Retrieve 凭据之前注入策略，并验证过期后刷新；只处理 Elasticsearch 业务 Transport 或 CloudWatch 公共凭据组件都会漏掉它。

阿里云 Tea 在 SDK 内部建立 Transport 和代理 Dialer，业务端设置一个外层 HTTP Client 不一定有效。需要核对当前依赖暴露的真实 hook、缓存和重建行为；缺少接口时采用受支持的依赖版本或最小上游补丁。禁止修改 Tea 的全局客户端缓存、全局环境或底层全局 resolver 作为产品修复。

认证专用的 Kerberos KDC、OAuth、身份联合、签名凭据刷新网络应单列子项。HTTP 业务请求成功且无 AAAA，不能据此认定认证链路已完成。

## 旁路和相关子系统

为避免本轮再次遗漏，将以下入口记入总清单，但与本轮 HTTP/SDK 两类分批交付：

| 入口 | 后续动作 |
| --- | --- |
| gNMI ICMP、SNMP ping、ping | 在探测开始前选择地址族，保留明确 IPv6 探测；特别注意 NewPinger 可能在构造时已解析 |
| net_response、ZooKeeper、HAProxy socket、PHP-FPM FastCGI、x509_cert SMTP/UDP | 同时处理 Resolve*Addr 和拨号入口；保留 Unix 和协议差异 |
| LDAP、WHOIS、Modbus、Oracle、SNMP 等连接型 SDK | 补充精确依赖接口清单；Modbus TCP/RTU-over-TCP 等模式分别检查，串口模式不变 |
| 内置 Prometheus 子系统 | 与 `inputs/prometheus` 分开：抓取、服务发现、通知、远程写入及其认证客户端分别适配 |

内置 Prometheus 已有可核对的部分入口：common/config 的 `WithDialContextFunc`、scrape 的 `HTTPClientOptions`、discovery 的 `HTTPClientOptions`。远程写入还包含独立的 round-robin DNS 和认证包装，需要继续确认所用版本的逐实例扩展能力，不能修改全局默认 Transport 来覆盖。

## 分阶段交付及完成门槛

| 阶段 | 改造内容 | 完成门槛 |
| --- | --- | --- |
| 第一阶段 | 全部自建 HTTP 入口及其装饰器、工厂、重建路径 | 清单中每个入口有接入证据和测试结果，额外鉴权或发现路径有单独状态 |
| 第二阶段 | MySQL、PostgreSQL、Redis/Sentinel、MongoDB、Kafka 指标及辅助发现、Docker/vSphere SDK | 真实协议 fixture 验证、节点变化/池补连/重建通过；MongoDB 的 SetDialer 与 SetHTTPClient、OCSP 和认证刷新均覆盖，默认配置兼容 |
| 第三阶段 | SQL Server、gRPC、Google/AWS/阿里云认证链路 | 解析 hook 和依赖阻塞项落地；gRPC 的原生代理选择、CONNECT、认证、代理 DNS 与 first hop IPv4 通过专项回归，不能只验 Dialer |
| 第四阶段 | 旁路协议和内置 Prometheus 专项 | 独立清单及验收全部完成后，再评估扩大整体产品保证 |

各阶段可以独立 PR 交付，但需维护累计状态，禁止用“公共封装已完成”替代调用方验收。清单状态统一为“待实现”“依赖阻塞”“待验收”“已验收”“明确例外”；例外必须写出触发条件、用户可见行为和理由。

## 回归测试和验收设计

### 共同测试矩阵

| 场景 | 验收目标 |
| --- | --- |
| 未设置、空值、false、其他值 | 与基线地址族、认证、超时和协议行为一致 |
| true 与 A/AAAA 双记录域名 | 实际连接成功，受控目标地址解析的 AAAA 为 0 |
| IPv6-only 域名 | 普通连接在 true 时失败且不回退；默认模式正常支持 |
| 显式 tcp6/udp6、IPv6 专项监控 | 保留显式配置语义 |
| IPv4 字面量、Unix、named pipe | 无额外目标 DNS；原通信方式正常 |
| 连接复用、服务端断连、池扩容、实例重建 | 各路径均遵循策略；不人为关闭 keep-alive |
| 多 A 记录、首地址失败 | 保留地址尝试和故障切换，不只取第一条记录 |
| HTTPS/TLS 与重定向 | 校验可信/不可信证书、SNI、Host、跨主机重定向和 HTTP/2；不以跳过校验代替验收 |
| 代理与认证 | 本地 first hop 的解析受控；目标域名传递、认证和明确例外符合设计 |
| context、超时、本地接口 | 取消和超时有效；地址族不兼容时明确失败 |

只统计与测试目标相关的 A/AAAA，SRV、TXT、PTR 单独计数。不得将“查询 AAAA 后忽略响应”记为通过。

### SDK 专项用例

- PostgreSQL：通过真实 pgx 建连验证主地址与 Fallbacks、TLS、Unix socket、CancelRequest；确认 LookupFunc 比 DialFunc 更早生效。
- MySQL/SQL Server：使用协议 fixture 或隔离测试服务，验证默认端口、参数绑定、认证、TLS、实例发现、路由和多地址；SQL Server 必须能检测驱动内部预解析。
- Redis/Sentinel：使用受信和不受信 TLS 服务验证握手未被裸 Dialer 绕过，覆盖 Sentinel 发现及重连。
- MongoDB：通过 SetDialer 和 SetHTTPClient 双入口，验证普通 URI、SRV/TXT、拓扑、心跳、池补连，以及冷缓存 OCSP responder 的真实请求、缓存/staple、Good/Revoked/soft-fail；验证 SDK 认证/元数据首次请求、过期刷新、重新认证、重建及私有 HTTP 池清理。分别统计数据库、OCSP、认证域名的 A/AAAA 和实际请求次数；不能通过关闭发现、OCSP 或证书校验来通过测试。
- Kafka：复用 mock broker 和 canonical DNS fixture，检查 bootstrap、metadata 新 broker、coordinator、重建；辅助 ZooKeeper 单独测试。
- gRPC：除 passthrough、dns 多地址、ResolveNow、service config、原 authority TLS 和取消外，必须验证 HTTPS_PROXY/https_proxy、NO_PROXY 的域名/IP/CIDR/localhost、WithNoProxy、WithLocalDNSResolution、带 Basic 认证 CONNECT、错误凭据/407、非成功响应、代理不可达、IPv6-only 代理和握手取消。代理模式只允许预期 CONNECT 流量到达代理，设置禁止直连的后端以检测绕过；默认远端解析时本地目标 A/AAAA 均为 0，代理 first hop 为 IPv4 且代理 AAAA=0。失败时不得直连，策略关闭时与基线代理行为一致。环境代理缓存场景用独立子进程隔离。
- 云 SDK：分别用本地模拟 API、STS/OAuth 和元数据服务，让 token 过期并触发刷新；无需真实账号和生产凭据即可完成 DNS 回归。
- Docker/vSphere：保留 API 协商、环境变量配置、TLS/root CA、会话续期及本地 socket 行为；Windows named pipe 在 Windows 执行。

### 环境和 CI

普通回归使用本地 DNS fixture 与临时监听端口，避免公网 DNS。修改全局 resolver 或进程环境的测试必须隔离且不并行；第三方 SDK 内部忽略默认 resolver 的场景，使用独立子进程或隔离系统解析器，不能让测试意外访问公网。

Linux 验收可复用 `10.129.0.77`，在独立网络/挂载命名空间内验证发布 Go 版本、纯 Go 与可用的 cgo/libc 路径。保留搜索域和 ndots 对照，分别统计目标域名、first hop、认证和发现流量。不修改服务器的主机 DNS 配置。

CI 应包含 Go 1.25 的受影响包 race 测试、golangci-lint 2.12.2、跨平台完整构建及所需的 `no_logs` 构建。标准 Context 回调优先；读取旧 Dial/DialTLS 所需的兼容代码保留局部有理由的抑制，避免重现 SA1019，禁止关闭整个 staticcheck。

## 文档和发布要求

每个 PR 同步插件 sample.conf/README、公共开关说明及累计清单。说明 `PREFER_IPV4_STACK` 的新增调用方、IPv6-only 目标的变化、认证/代理边界、未完成依赖和回滚方式。

插件默认行为、指标和标签格式应保持兼容。发现无关的 TLS、认证或配置错误时单独记录并处理，避免在网络策略迁移中混入未经验证的行为修正。

最终发布保证以“已验收”的具体插件和路径为单位。全部动态发现、认证刷新及旁路协议完成前，持续保留未覆盖清单，避免把两类客户端的改造描述成整个进程 AAAA 查询归零。
