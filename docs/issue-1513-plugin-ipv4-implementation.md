# Issue 1513 插件 IPv4 策略累计实施记录

- 分支：`fix/ipv4-dns-policy-1513`
- 日期：2026-11-12
- 设计依据：[插件 IPv4 策略方案](issue-1513-plugin-ipv4-policy-plan.md)
- 代码状态：第一至第四阶段均已接入；vSphere 连接池清理问题已修复。以下按具体路径记录本地验收及明确例外，不以代码接入替代部署验收。

仅精确字符串 `PREFER_IPV4_STACK=true` 启用。客户端创建时捕获策略，修改后需重启。普通 IPv6-only 目标失败，不回退 IPv6；显式地址族、IPv6 专项探测、主动 DNS 查询、Unix socket、named pipe、串口与监听端保留配置。代理约束本地 first hop，代理端 DNS 不属于本地保证。

## vSphere 连接池修复

不再把克隆的 Transport 替换到 `soap.Client.Transport`。对 SDK 自己持有的 Transport 原地配置拨号策略，使实际 HTTP Client 和 SDK 的 `c.t` 始终指向同一个连接池。最小 govmomi 补丁把此配置传给 `NewServiceClient` 和证书登录的 SDK tunnel，并按父客户端跟踪这些 Transport。

`govmomi.Client.Logout` 原有的延迟清理现在能关闭整个组的空闲连接；插件 `ClientFactory.Close` 沿用该生命周期。构造失败也清理已建立的连接。回归测试观察服务端实际连接关闭，覆盖 SDK 清理、Logout 成功/失败、插件关闭成功/失败、重复关闭、派生客户端，以及首次 SOAP 请求失败。测试不以只验证首请求或 Transport 字段代替池释放。

## 接入和本地验收

“已验收”仅指表中明确列出的本地 fixture 路径。真实集群、各生产身份服务和不同系统 resolver 的完整部署矩阵仍需发布前验收。

| 路径 | 状态 | 代码和本地证据 |
| --- | --- | --- |
| 自建 HTTP 清单 | 已验收 | nginx、nginx_upstream_check、apache、HAProxy HTTP、rabbitmq、tomcat、logstash、influxdb、clickhouse、Jolokia 公共客户端、jenkins、redfish、tengine、NATS 监控 HTTP、xskyapi、appdynamics、cadvisor、Kubernetes、Prometheus 插件抓取、consul、emc_unity、PHP-FPM HTTP、traffic_server、bind HTTP、RocketMQ offset、Hadoop HTTP、Huatuo、x509_cert HTTP、supervisor、node_exporter supervisord 已接入；构造入口覆盖域名、IPv6-only、策略捕获、复用和重连，保留 TLS/代理/超时配置 |
| Elasticsearch HTTP | 已验收 | 先配置底层 Transport，再添加 API Key/AWS 签名；HTTP/HTTPS 认证回归通过；独立 STS 入口在加载配置前接入 |
| CloudWatch 与 AWS 公共凭据 | 已验收 | 业务 HTTP 保留原设置；root config 注入可继续被 SDK 克隆的 BuildableClient，最终 Transport 配置跨 defaults mode、CA 与 IMDS 超时调整保留；本地 STS 签名、过期凭据刷新及克隆后 DNS 测试通过 |
| Docker 指标/日志 | 已验收 | 共用 `pkg/dockerx`，在最终 host/TLS 后配置；实际 SDK 环境入口、显式 endpoint、协商、TLS 与本地 socket 用例通过；Windows 运行另列 |
| MySQL、PostgreSQL | 已验收 | MySQL 先归一化 DSN 再设置 tcp4 Connector；pgx 在 LookupFunc 先查询 ip4，保留 DialFunc。实际握手、TLS、PING、池补连；pgx fallback/CancelRequest/重建通过 |
| Redis/Sentinel | 已验收 | 通过 Network 保留 SDK 的 TLS-aware Dialer；认证、受信/不受信 TLS、PING 与断连后补连通过 |
| MongoDB | 已验收 | SetDialer/SetHTTPClient 双入口；私有 HTTP 池在 Disconnect 后清理，构造失败也清理；真实 wire 握手、PING、OCSP Good/Revoked/soft-fail、缓存、重建及策略捕获通过。完整拓扑/SRV/TXT/身份机制矩阵另列待验收 |
| Kafka、辅助 ZooKeeper、Kerberos | 已验收 | 日志和指标复用 `pkg/kafkax` 工厂，重建保留策略；mock broker 与 A/PTR/canonical 回归通过。Kazoo 获得 HostProvider/Dialer，保留原生地址轮换；KDC 在 UDP/TCP 解析前选择地址族，真实初始认证请求及重连拨号测试通过 |
| vSphere | 已验收 | 首次 SOAP 的 HTTPS/CA；父子客户端、Logout/插件关闭、失败与重复清理的实际连接释放通过，见上节 |
| SQL Server | 已验收 | 最小驱动补丁提供 Connector.LookupIP 和 NetworkPolicy；解析在多地址竞争前使用 ip4，SQL Browser UDP 同样受控。保留注册的 mssql Driver/查询处理模式；本地实际 TDS prelogin、重复连接及 DNS 回归通过 |
| Oracle | 已验收 | Connector 策略随 Session/redirect 传递，在原生超时选择后限定 TCP 网络；保留 Unix/TLS。实际 TNS connect 首包及 DNS 回归通过 |
| gRPC/gNMI | 已验收 | 私有原生 DNS builder 保留 SRV/TXT、service config 与 resolver 生命周期；新增 native network/proxy resolver hook，不用裸 WithContextDialer。真实 DNS/passthrough、TLS RPC；HTTPS_PROXY/小写、Basic CONNECT、NO_PROXY 域名/IP/CIDR/localhost、WithNoProxy、本地/远端解析、407/502、代理不可达/IPv6-only、取消均通过。代理用例子进程隔离环境缓存，并检查失败无直连、远端解析时本地目标 A/AAAA 均为 0 |
| Google Monitoring 凭据 | 已验收 | gRPC 与新旧 ADC/OAuth/STS/元数据的逐客户端策略同组创建；保留 metadata 及订阅不同超时；X509 workload/mTLS 与 GDCH 新建客户端仍应用策略。实际 OAuth 过期刷新、新旧 ADC metadata 请求、用户 mTLS 客户端证书/CA 保留及 DNS 回归通过；实验性 S2A/DirectPath 例外见下 |
| 阿里云 CMS 两种客户端 | 已验收 | Tea 私有池和最终 Transport hook，不改全局缓存；SOCKS first hop 在创建代理 Dialer 前选族。实际 CMS v1/v2 SDK 签名请求、复用、实例重建、双池关闭及 SOCKS 首跳/远端解析通过 |
| LDAP/LDAPS、Modbus、net_response、ZooKeeper、SNMP、ping | 已验收 | LDAP/LDAPS 实际拨号和 CA；Modbus TCP/RTUoverTCP/ASCIIoverTCP 的连接及重连；net_response TCP/UDP；ZooKeeper TCP/TLS/SNI；SNMP connected/unconnected UDP；native ping 首次解析及 exec 参数均通过，显式 IPv6 保留 |
| WHOIS、LDAP StartTLS、HAProxy socket、PHP-FPM FastCGI、x509_cert SMTP/UDP、gNMI ICMP、SNMP ping | 待验收 | 代码均已接入解析/拨号入口；保留原协议、TLS 和超时。WHOIS 包括后续 referral；FastCGI Unix 与串口不改变。本地编译/vet 通过，完整协议交互和部署验收尚未全部执行 |
| 内置 Prometheus | 已验收 | 与 inputs/prometheus 分开：scrape、discovery、notifier、remote read/write 均接入；覆盖独立 OAuth、SigV4、Azure AD、Google IAM 和发现 SDK 的 HTTP。remote round-robin 在 lookup 前选族并保留原随机地址选择；agent/TSDB 两种存储的实际抓取到远程写入、OAuth 刷新、HTTP SD 与通知通过 |

## 可重现依赖交付

缺少公开 hook 的依赖通过 root `go.mod` 的本地 `replace` 固定源码交付，保留原版本，不依赖修改忽略的 vendor 目录。19 个模块的保留源码、许可证、上游版本/Go 校验和、修改文件清单及审查补丁位于 [third_party](../third_party/README.md)。`go mod vendor` 可重建补丁后的依赖；module mode 使用同一源码。

[核验脚本](../scripts/verify-network-patches.py) 将保留源码逐文件对照 Go 校验的固定上游版本，验证所有差异与提交的 patch 一致。CI 先核验补丁，再执行 Go 1.25 race 回归。

Prometheus AWS discovery 自动 region/IMDS 查询从 YAML 解码延迟到 discoverer 创建，以便在网络活动之前获得实例策略；未指定且无法解析 region 时，错误在创建 discoverer 时报告。该错误时机变化是本次依赖适配的明确兼容变化。

Elasticsearch 同时修复了与 Transport 重排直接相关的原有构造错误：TLS 覆盖 API Key 装饰器、AWS 分支向未初始化的 ins.Client 赋值。原认证配置仍保留。

## 明确例外与尚待部署验收

- **明确例外**：主动 DNS/AAAA 监控、明确 tcp6/udp6 或 IPv6 探测、本地 socket/串口/named pipe 和监听端，不按普通域名拨号处理；NATS 此次入口是监控 HTTP，不泛化为全部 NATS 协议。
- **明确例外**：代理端解析、任意用户自定义 resolver/dialer/TokenProvider、外部 credential executable 或自定义凭据回调自行发起的网络，保留其契约。Kafka 自定义 proxy 与 canonical bootstrap 的不支持组合明确报错；MongoDB 不透明 HTTP RoundTripper 开启策略时明确拒绝。
- **明确例外**：Google 实验性 S2A、DirectPath 使用独立私有网络策略，开启本策略的组合返回清晰的构造错误；不静默关闭原模式或回退双栈。普通证书 mTLS 保留。
- **待验收**：完整 SQL 登录/参数绑定/SQL Browser 路由、多 A 故障切换、MongoDB SRV/TXT/拓扑/staple/MONGODB-AWS/OIDC 重新认证、Kafka coordinator/新节点、vSphere 完整会话续期，以及各云 SD 的实际身份/发现矩阵。这些路径的代码已接入，未用本地首包或通用客户端测试冒充全部生产验收。
- **待验收**：Linux cgo/libc、搜索域/ndots 对照、真实集群与生产身份系统；本地没有 Windows 运行环境，named pipe 双开关运行测试交由 Windows CI。跨平台编译不等同于运行验收。

## 验证记录

测试使用本地 DNS、临时端口和协议 fixture，不需要生产账号。

- Go 1.25.10 / Darwin arm64：受影响范围 60 个有测试的包 `go test -race -count=1` 通过，`go vet` 通过。随后补充的 vSphere 实际 `Instance.Drop()` 及 SNMP 显式 IPv6/zone 解析回归分别复测通过。
- Go 1.25.10：`-mod=readonly` 下 vSphere、gRPC、AWS、Google 新旧凭据、CMS、Kafka/KDC、内置 Prometheus 的 race 回归通过，验证 module mode 使用相同补丁源码。
- Go 1.25.10：`no_logs` 下共享 Kafka 适配和指标 exporter 的 race 回归通过。
- golangci-lint 2.12.2：受影响包新增/修改行（包含未跟踪的新 Go 文件，`--tests=false`）为 `0 issues`。
- Go 1.25.10、`CGO_ENABLED=0`：完整主程序的 Linux amd64、Linux arm64、Windows amd64，以及 Linux amd64 的 `no_logs` 构建全部通过。
- 19 个 SDK 固定上游版本、校验和、保留源码和审查补丁逐项核验通过；`go mod vendor` 重建通过。
- CI YAML 与 folded 测试命令解析通过；应用 Go 文件的 gofmt、差异空白及文档相对链接检查通过。
- CI 包含受影响包 race、补丁核验、完整 no_logs 构建和 Windows named-pipe 开关矩阵；Windows 运行结果尚未在本地执行。

扩大到非本次改动范围时，Elasticsearch collector 的既有 `TestClusterHealth`/`TestHealthReport` 指标期望失败；Darwin 上对整个 `./...` lint 还会命中未改动 `node_exporter/collector/other_os` 的独立 BSD 包类型错误。本次未混入这些无关修正，上述验证结果明确限定到受影响包。

撤销环境变量或设置 `false` 后重启，恢复默认地址族选择。独立依赖 hook 在未配置时沿用上游路径；上述明确记录的 region 错误时机和 Elasticsearch 构造错误修正，以及下述 RFC 4757 编码修复仍保留。不能将本次交付描述为整个进程 AAAA 查询归零。

## 评审修正：tidy 闭包、Azure 认证、Docker TCP 发现

- 本地 gRPC/AWS 替换补齐固定版本的 `test/bufconn`、`interop/grpc_testing`、`interop/grpc_testing/core`、`internal/awstesting` 和 `internal/awstesting/unit`。这些包被依赖的测试导入，不能只按运行时 vendor 裁剪。已执行实际 `go mod tidy` 同步依赖分类和校验和；CI 新增 `go mod tidy -diff`，补丁核验也显式检查辅助包，且不会向根 go.sum 重新写入被 tidy 移除的上游校验和。
- Azure 服务发现先创建实例私有的原生 SDK 凭据客户端，再注入所有认证方式。新增 azcore 小型固定补丁保留原生 TLS、HTTP/2、代理和超时默认值，只在解析前选择地址族；凭据池跨 refresh 复用，发现停止时释放。OAuth、Managed Identity、Workload Identity、DefaultAzureCredential 通过实际 ARM 调用和 token 过期刷新验证，认证域名 A>0、AAAA=0；没有替换全局 SDK 客户端。
- Docker/Swarm 的 `tcp://` 在最终 host 配置后包装 SDK 原生 DialContext，保留其超时及 KeepAlive；原有 HTTP/HTTPS 认证/TLS 配置和 Unix/named pipe 分支保留。Docker、Swarm nodes/services/tasks 的真实发现请求、开关对照、重连、策略捕获及 Unix 回归通过。

Azure 的专项回归位于本地 Prometheus 模块，CI 使用 `-mod=readonly` 显式执行 `github.com/prometheus/prometheus/discovery/azure`，避免生成 vendor 不携带依赖测试文件而漏跑。

本次复核结果（Go 1.25.10）：`go mod tidy -diff` 在核验脚本及 `make vendor-ci` 前后均无差异，核验脚本执行前后根 go.mod/go.sum 字节保持一致；61 个测试包的完整 `-mod=readonly -race -count=1` 回归通过，受影响包及 Azure 专项 vet 通过，新增/修改行 lint 为 0 issues。Linux amd64/arm64、Windows amd64 与 Linux amd64 no_logs 的完整构建全部通过。远端 GitHub Actions 尚未运行。


## 2026-11-12 全面自查补充

- **AWS 容器凭据**：`AWS_CONTAINER_CREDENTIALS_FULL_URI` 的 HTTP hostname 校验在创建 provider 前独立调用双栈解析，endpoint provider 又不继承根 `cfg.HTTPClient`。新增固定版本 `aws-sdk-go-v2/config` 适配，提供逐次加载的 lookup hook；公共 AWS 工厂同时注入 IPv4 lookup 与 endpoint credential HTTP client。保留原生 loopback/ECS/EKS 允许地址规则、所有返回地址逐一校验、token-file 读取和 SDK 超时默认值。过期凭据连续刷新、token-file 更新、创建时捕获策略、开关对照、公共地址拒绝和 IPv6-only 校验均通过。
- **AWS 服务发现**：EC2、ECS、ElastiCache、Lightsail、MSK、RDS 在各自实际 `LoadDefaultConfig` 前注入凭据策略；不能只覆盖自动 region 查询。新增显式 region 的 EC2 真实凭据获取、SigV4 API 请求和重复发现回归，开启时 A>0、AAAA=0。SDK option slice 按 discoverer 复制。
- **Azure AD 远程写入**：独立认证客户端改用 azcore 原生 HTTP 客户端工厂，保留 TLS/HTTP2/超时默认值并注册到实例所有者；停止时清理凭据池。实际 Managed Identity token 刷新、Bearer 认证头、DNS 开关对照和连接关闭通过。Azure SD 的四种认证专项回归继续通过。
- **CI**：移除 `scripts/**` 的路径忽略，校验脚本单独变更也会触发检查；vet 脚本启用 `pipefail`，`go list` 失败不再被后续管道掩盖；版本检查从 `go.mod` 读取最低版本，并移除 `go` 前缀后按数值比较。旧脚本错误接受 Go 1.20 已复现；新检查拒绝低版本和当前最低版本的 rc，接受 Go 1.25.0/1.25.10/1.26，并验证前置命令失败会传播。
- **依赖交付**：19 个固定版本 snapshot/patch 核验通过。新增 AWS config 保留上游 go.mod/go.sum 与 LICENSE；真实 tidy 未发现新的缺失辅助包，仅移除了该本地替换对应的上游校验和。核验与 vendor 重建前后再次检查 tidy 闭包；不通过手动补回根校验和掩盖差异。
- **路径审查**：逐个检查 Prometheus 发现客户端以及 Google/AWS/Azure 的身份刷新入口；OpenStack 在 Authenticate 前注入客户端，OVH 构造本身不联网，STACKIT 服务账号认证继承已配置 Transport，Scaleway 使用传入 HTTP client。当前注册的 Kubernetes GCP auth-provider 是移除功能的错误 stub，exec 凭据仍属于外部进程边界。MongoDB 默认构建的 MONGODB-AWS/OIDC 接收配置的 HTTP client；额外启用 `gssapi` build tag 时，系统 Kerberos 库自行发起的网络属于原生库边界，不纳入 Go Transport 的保证。

本轮 Go 1.25.10 的 61 个测试包 `-mod=readonly -race -count=1`、受影响包 vet 和新增/修改行 lint（0 issues）通过；Linux amd64/arm64、Windows amd64 和 Linux amd64 no_logs 完整主程序构建通过。远端 GitHub Actions 及 Windows 实机运行尚未执行。额外尝试在 macOS 上以 Linux/`CGO_ENABLED=0` 跑全目录 lint/vet 时，NVIDIA 的独立包因缺少 cgo 类型而失败；这不是 Linux CI 原生 cgo 环境的执行结果，也未改动这些无关包。

## PR #1519 Copilot 复核：RFC 4757 key usage 编码

[Copilot 评论](https://github.com/flashcatcloud/categraf/pull/1519#discussion_r4545178700) 指出 gokrb5 的 `UsageToMSMsgType` 使用了变长 `PutUvarint`。已确认该文件与固定上游 v8.4.4 完全一致，是随源码快照保留的依赖既有缺陷。[RFC 4757 第 3–5 节](https://www.rfc-editor.org/rfc/rfc4757.html#section-3) 要求四字节小端整数；usage=1024 原先得到 `80 08 00 00`，应为 `00 04 00 00`，而最大 uint32 会在四字节缓冲区内触发越界 panic。

现改用 `binary.LittleEndian.PutUint32`，保留 3→8、9→8、23→13 的协议映射，并同步 `gokrb5.patch` 和 `network-patches.json`。该正确性修复不受 IPv4 开关影响。

回归放在 CI 已执行的根模块 `pkg/kafkax/rfc4757_test.go`，覆盖映射、127/128/255/256、1024、四/五字节变长编码边界和最大 uint32；另以独立生成的 RFC 固定 checksum/密文验证解密，并用标准库 HMAC/RC4 独立校验随机加密输出。修复前已实际复现编码、checksum、解密、加密互操作错误及越界 panic，避免仅用双方共享错误编码的加解密 round trip 作为依据。

本次修正验证：Go 1.25.10 下 Kafka 相关包在 module/vendor 两种模式的 race 回归通过，no_logs 下共享工厂和指标 exporter 的 race 回归通过，相关包及 RFC 4757 包 vet 通过；19 个依赖快照核验、tidy 前后无差异和 vendor 重建通过，根 go.mod/go.sum 保持不变。Linux amd64/arm64、Windows amd64 与 Linux amd64 no_logs 的完整主程序构建通过。

## PR #1519 CI 复核：大型 PR 的 lint 差异获取

[远端运行 #605](https://github.com/flashcatcloud/categraf/actions/runs/43137441778) 的 Code Analysis 日志确认：GitHub diff API 因 PR 超过 300 个文件返回 HTTP 406（`too_large`），lint action 的 `only-new-issues` 因此无法获得 patch，回退为全量检查。附件中的 168 条报告位置均不在该 PR 新增行上。该次运行的 Commit Lint、Go Build Check、Plugin IPv4 DNS Regression 及 Windows named-pipe 两种开关均已通过。

GoLint 现拉取完整历史，并在运行前验证 PR base commit 存在；通过 `--new-from-rev=<PR base SHA>` 由本地 Git 提供差异，关闭 action 的 API patch 获取。PR 仍检查新增问题，手动 workflow_dispatch 保留原有全量检查；baseline 缺失会明确失败，不会回退为全量检查或跳过 lint。

本地 Go 1.25 检查中，受影响包对真实 PR base 的 lint 为 0 issues，actionlint 工作流检查通过。独立临时仓库回归验证：base 中已有的 lint 错误被过滤，而新增行的相同错误仍导致 lint 失败。
