# Categraf Dependabot 安全告警分析与修复方案

- 整理日期：2026-10-29
- 状态：PostgreSQL 与 Docker SDK 两批迁移均已实现并完成本地/隔离环境验证；待 PR CI 及 GitHub 重新分析
- 评审修订：补全 Prometheus Docker/Swarm 服务发现依赖闭包，以及 simple protocol 的真实用户配置入口。
- 告警来源：[Dependabot 汇总页](https://github.com/flashcatcloud/categraf/security/dependabot)
- 核对方式：通过已登录 Chrome 读取 6 条告警，并核对当前仓库代码、依赖元数据和上游公告。

## 1. 结论与修复顺序

6 条告警分为两组，按两个独立提交实施，便于分批审查：

1. **优先迁移 PostgreSQL 驱动**：从 `github.com/jackc/pgx/v4` 迁移到 `github.com/jackc/pgx/v5 v5.9.2` 或经核验的更高修复版本，处理 #50 和 #69。
2. **迁移独立 Docker SDK 及其完整依赖闭包**：同时处理本仓库三个直接使用目录和 Prometheus Docker/Swarm 服务发现，替换 `github.com/docker/docker` 整体模块，处理 #75、#77、#66 和 #76。完整依赖路径盘点与 Prometheus 迁移路线确定是实施前置条件，不能只在完成三个目录迁移后才检查。

告警页面中的 `Patched version: None` 表示对应旧模块路径没有可直接升级的修复版本，不代表整个上游项目没有修复方案。单纯提高旧路径下的版本号，不能解决本次全部告警。

必须区分两个验收目标：

- Categraf 仓库：移除受影响的旧依赖，验证实际调用路径和功能兼容性。
- 运行环境：Docker daemon 是独立服务，更新 Categraf SDK 不会修复宿主机上的 Docker Engine 漏洞。

## 2. 当前依赖与使用范围

迁移前核对时 `go.mod` 中的相关版本（PostgreSQL 实施结果见第 4.4 节）：

| 模块 | 当前版本 | 用途 |
| --- | --- | --- |
| `github.com/jackc/pgx/v4` | `v4.18.2` | PostgreSQL 采集驱动 |
| `github.com/jackc/pgconn` | `v1.14.3` | pgx v4 引入的连接层依赖 |
| `github.com/jackc/pgproto3/v2` | `v2.3.3` | pgx v4 依赖链中的 PostgreSQL 协议库 |
| `github.com/docker/docker` | `v28.5.2+incompatible` | Docker 客户端和 API 类型 |
| `github.com/prometheus/prometheus` | `v0.311.3` | 内嵌 Prometheus，服务发现会再次引入旧 Docker SDK |

主要代码位置：

- PostgreSQL：`inputs/postgresql/postgresql.go`、`inputs/postgresql/postgresql_test.go`。
- Docker 指标：`inputs/docker/`，包括客户端封装和 Stats 指标解析。
- Docker 日志：`logs/util/docker/`，包括容器、镜像、事件和日志流处理。
- Keepalived 容器采集：`inputs/keepalived/types/container/`，包括 Docker exec。
- Prometheus 服务发现：`prometheus/prometheus.go` 的 `discovery/install` 注册入口，以及第三方模块的 `discovery/moby/`。

已用 `go list -mod=vendor` 核实以下传递依赖链：

```text
flashcat.cloud/categraf/prometheus
  -> github.com/prometheus/prometheus/discovery/install
  -> github.com/prometheus/prometheus/discovery/moby
  -> github.com/docker/docker/client
```

`discovery/moby` 中的 `docker.go`、`dockerswarm.go`、`network.go`、`nodes.go`、`services.go` 和 `tasks.go` 仍导入旧 SDK 或 API 类型。仅修改本仓库三个直接使用目录不能移除旧模块。即使运行配置没有启用 Docker/Swarm 服务发现，完整构建仍通过注册入口包含该依赖；裁剪构建也不能代替默认完整构建的依赖验收。

本次代码审查未发现 Categraf 调用 Docker 归档上传/查询接口或插件安装接口，也未发现它启动 Docker daemon。Docker 相关结论属于当前采集路径的源码分析，仍需在实施阶段以调用图扫描和二进制扫描进一步确认，不能仅凭模块名称判断可利用性。

## 3. 六项问题及各自修复方案

| 告警 | CVE | 级别 | 问题 | 旧模块受影响范围 |
| --- | --- | --- | --- | --- |
| [#50](https://github.com/flashcatcloud/categraf/security/dependabot/50) | CVE-2026-32286 | High | PostgreSQL DataRow 负字段长度导致 panic | `pgproto3/v2 >= 2.0.0, <= 2.3.3` |
| [#69](https://github.com/flashcatcloud/categraf/security/dependabot/69) | CVE-2026-41889 | Low | simple protocol 对 dollar-quoted SQL 的参数替换混淆 | `pgx/v4 <= 4.18.3` |
| [#75](https://github.com/flashcatcloud/categraf/security/dependabot/75) | CVE-2026-41567 | High | 压缩归档上传时执行容器内解压程序，获得 daemon 权限 | `docker/docker <= 28.5.2` |
| [#77](https://github.com/flashcatcloud/categraf/security/dependabot/77) | CVE-2026-42306 | High | docker cp 竞态将 bind mount 重定向到宿主机路径 | `docker/docker <= 28.5.2` |
| [#66](https://github.com/flashcatcloud/categraf/security/dependabot/66) | CVE-2026-33997 | Moderate | Docker 插件安装权限校验存在 off-by-one 错误 | `docker/docker < 29.3.1` |
| [#76](https://github.com/flashcatcloud/categraf/security/dependabot/76) | CVE-2026-41568 | Moderate | docker cp 路径竞态在宿主机创建空文件或目录 | `docker/docker <= 28.5.2` |

### 3.1 #50：PostgreSQL DataRow 解码拒绝服务

**触发条件与影响**：恶意或被入侵的 PostgreSQL 服务端返回字段长度小于 `-1` 的 DataRow，旧版 `DataRow.Decode` 未拒绝该长度，可能触发切片越界 panic，影响采集进程可用性。该风险属于实际使用的 PostgreSQL 协议解码链。

**修复方案**：

- 将 PostgreSQL 插件迁移到 `pgx/v5 >= v5.9.2`。
- 使用 v5 内置的 `pgx/v5/pgproto3`，清理旧 `pgproto3/v2` 依赖链。
- 上游维护者已明确说明旧协议库结束维护，v5 对应实现不受此问题影响；本地核对的 v5.9.2 解码实现包含负长度校验。

**验证要求**：

- 构造字段长度为 `-2` 等非法值的 DataRow，断言返回错误且不发生 panic。
- 同时覆盖合法 NULL（`-1`）、空字段、截断字段及越界长度，防止新增保护破坏正常结果。
- 验证真实 PostgreSQL 采集正常，错误服务端响应不会导致整个采集进程崩溃。

参考：[漏洞公告](https://github.com/advisories/GHSA-jqcq-xjh3-6g23)、[上游维护者说明](https://github.com/jackc/pgx/issues/2507)、[v5.9.2 解码实现](https://github.com/jackc/pgx/blob/v5.9.2/pgproto3/data_row.go)。

### 3.2 #69：simple protocol 参数替换导致 SQL 注入

**触发条件**：同时使用 simple protocol、包含 dollar-quoted 字符串的 SQL、字符串内存在类似占位符的文本，并且攻击者能够控制相关绑定参数。

**当前代码情况**：`IsPgBouncer` 标记为 `toml:"-"`，仓库中未找到将其设置为 true 的用户配置路径，因此不能把该条件分支作为当前 PgBouncer 用户入口的证据。

已确认的真实入口是 `[[instances]].address` 连接字符串。pgx v4 的 `ParseConfig` 支持其中的 `prefer_simple_protocol` 参数：

```toml
[[instances]]
address = "host=127.0.0.1 user=test dbname=test sslmode=disable prefer_simple_protocol=true"
```

URI 形式也可进入相同模式：

```toml
[[instances]]
address = "postgres://test@127.0.0.1/test?sslmode=disable&prefer_simple_protocol=true"
```

该路径是 TOML 加载 → `Instance.Address` → `Instance.Init` → `pgx.ParseConfig` → `stdlib.RegisterConnConfig`。正常配置加载下 `IsPgBouncer` 仍为 false，但解析出的 `PreferSimpleProtocol` 可以为 true。另外，`prepared_statements = false` 会被当前 `Init` 的默认分支覆盖为 true，不能将它当作可靠的 simple protocol 开关。

目前采集代码使用 `db.Query(query)`、`db.QueryContext(ctx, metricConf.Request)` 等不带绑定参数的调用，尚未发现满足漏洞全部条件的攻击路径。不应将“配置允许 simple protocol”直接等同于“已确认存在可利用注入”。

**修复方案**：

- 与 #50 在同一 PR 中迁移到 `pgx/v5 >= v5.9.2`，该版本修复了此参数替换问题。
- 为用户真实使用的 `address` 参数提供兼容处理：将旧 `prefer_simple_protocol=true` 映射为 `DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol`，并从发送给 PostgreSQL 的运行时参数中移除旧键。
- 支持 v5 原生的 `default_query_exec_mode=simple_protocol`，明确新旧参数冲突处理，具体规则见第 4.2 节。
- 首次迁移保留由连接配置明确选定的 simple protocol 行为；不以手动设置 `IsPgBouncer` 或直接修改该字段的 TOML 标签代替真实配置迁移。

**验证要求**：

- 在真实 PostgreSQL 测试库中执行包含 dollar-quoted 字符串与恶意绑定参数的回归用例，验证参数仍作为数据处理，测试表不受影响。
- 从上面的 URI 和键值形式 TOML 开始，经过真实配置加载与初始化后分别验证 PostgreSQL 直连和 PgBouncer；不能只在测试中构造 `Instance{IsPgBouncer: true}`。
- 保留现有 SQL mock 测试，但不能用它替代驱动参数替换和协议层测试。

参考：[漏洞公告及修复版本](https://github.com/advisories/GHSA-j88v-2chj-qfwx)、[pgx v5.9.2](https://github.com/jackc/pgx/releases/tag/v5.9.2)。

### 3.3 #75：压缩归档上传触发宿主机代码执行

**触发条件与影响**：向恶意容器调用 `PUT /containers/{id}/archive` 并上传特定压缩格式的归档时，受影响 daemon 可能从容器文件系统解析解压程序路径，使容器提供的恶意程序以 daemon 权限运行。普通默认发送未压缩 tar 的 `docker cp` 不满足此问题的全部条件。

**Categraf 修复方案**：迁移独立 Docker SDK，移除旧整体模块；保持现有采集功能范围，不引入归档上传能力。通过调用图/二进制扫描确认没有相关可达服务端实现。

**运行环境修复方案**：Docker Engine 的对应修复下限为 `29.5.1`。不能只升级 SDK 就宣布宿主机风险已消除；部署侧应选择包含修复的受支持补丁版本。在更新前，限制对归档上传接口的访问，并避免向不受信任容器上传压缩归档。

**验证要求**：SDK 迁移后的容器、镜像、事件和日志操作正常，采集流程不请求归档接口；真实 daemon 的修复由其版本与发行方补丁记录确认，不在生产主机执行宿主机代码执行 PoC。

参考：[漏洞公告](https://github.com/advisories/GHSA-x86f-5xw2-fm2r)、[Docker Engine 29.5.1](https://docs.docker.com/engine/release-notes/29/#2951)。

### 3.4 #77：bind mount 目标重定向

**触发条件与影响**：对带有卷挂载的恶意运行中容器执行 docker cp，或调用相应归档 API，容器内进程通过符号链接替换竞态，可能将临时 bind mount 指向宿主机任意路径，造成文件写入或拒绝服务。

**Categraf 修复方案**：与 #75 共用独立 SDK 迁移，验证当前采集路径不调用 `PUT /containers/{id}/archive`、`HEAD /containers/{id}/archive`，不依赖 daemon 的挂载实现。

**运行环境修复方案**：对应 daemon 修复下限为 `29.5.1`。更新前避免对不受信任的运行中容器执行相关拷贝操作，并限制归档接口权限。

**验证要求**：完成新 SDK 的功能回归和不可达性核对。若需要验证 daemon 的竞态修复，应使用独立、可销毁的隔离测试环境，不在共享测试主机或生产主机执行攻击用例。

参考：[漏洞公告](https://github.com/advisories/GHSA-rg2x-37c3-w2rh)、[Docker Engine 29.5.1](https://docs.docker.com/engine/release-notes/29/#2951)。

### 3.5 #66：插件安装权限校验绕过

**触发条件与影响**：安装恶意 Docker 插件时，daemon 的权限集合比较存在 off-by-one 错误，可能接受与用户批准集合不同的权限。不使用相关插件安装流程时，不满足此告警的触发条件。

**Categraf 修复方案**：同一批迁移独立 SDK，移除旧整体模块；确认 Docker 指标、日志、Keepalived 采集及 Prometheus Docker/Swarm 服务发现均不调用插件安装接口。

**运行环境修复方案**：使用 Docker 插件的环境应更新 daemon，对应修复下限为 `29.3.1`。更新前仅安装可信插件，并限制 daemon 管理权限。为同时覆盖另外三个 Docker 告警，统一选择包含 `29.5.1` 修复的受支持版本。

**验证要求**：依赖图不再包含旧模块，采集功能不依赖插件管理路径；部署侧单独确认 daemon 补丁状态。

参考：[漏洞公告](https://github.com/advisories/GHSA-pxq6-2prw-chj9)、[Docker Engine 29.3.1](https://docs.docker.com/engine/release-notes/29/#2931)。

### 3.6 #76：宿主机任意路径创建空文件或目录

**触发条件与影响**：docker cp 挂载点创建期间存在路径解析与符号链接替换竞态，恶意容器可使 daemon 在宿主机指定路径创建空文件或目录，影响主机服务可用性。此问题与 #77 的 mount 系统调用竞态相关，但属于不同阶段，需要分别跟踪。

**Categraf 修复方案**：共用独立 SDK 迁移，移除旧整体模块并核对归档接口不可达性。

**运行环境修复方案**：对应 daemon 修复下限为 `29.5.1`。上游通过将挂载点创建操作限制在容器根目录内修复，不应在 Categraf 中自行复制或修改 daemon 的文件系统实现。

**验证要求**：SDK 功能回归、依赖图检查，以及部署侧 daemon 版本/补丁确认。攻击复现仅限可销毁的隔离环境。

参考：[漏洞公告](https://github.com/advisories/GHSA-vp62-88p7-qqf5)、[Docker Engine 29.5.1](https://docs.docker.com/engine/release-notes/29/#2951)。

## 4. 第一批实施细节：PostgreSQL 驱动迁移

### 4.1 代码与依赖调整

1. 将 `inputs/postgresql/postgresql.go` 中 `pgx/v4`、`pgx/v4/stdlib` 的导入改为 v5。
2. 以 `v5.9.2` 为已核验的最低统一修复目标。其 Go 要求为 1.25，与当前项目 `go 1.25.0` 兼容。
3. 增加可测试的连接配置解析/归一化入口，例如 `parseConnectionConfig(address)`，并由实际 `Instance.Init` 调用；按第 4.2 节处理旧参数及 v5 查询模式，保持 `database/sql` 使用方式。
4. 核对直连默认模式、连接字符串参数和驱动注册行为，避免 v4/v5 同时注册 `pgx` 驱动产生冲突。
5. 清理旧驱动链中的 `pgconn`、`pgproto3/v2` 等不再使用的依赖。若仍被其他模块引入，使用依赖图定位来源并处理，不能仅删除一行 `require`。
6. 更新 `go.mod`、`go.sum`，按仓库现有流程刷新 vendor，避免构建继续使用旧源码。

### 4.2 用户配置入口与参数迁移规则

配置迁移以 `address` 为准，不假定 `IsPgBouncer` 可以由用户设置。当前 `prepared_statements` 字段与 PgBouncer 相关注释存在行为不一致，应更正文档，不把修复该历史开关作为本次安全迁移的隐含前提。

| 用户连接配置 | 迁移后行为 |
| --- | --- |
| 没有协议模式参数 | 保持 v5 默认查询模式，不根据不可达的 `IsPgBouncer` 分支自动启用 simple protocol |
| 只有 `prefer_simple_protocol=true` | 兼容映射为 `QueryExecModeSimpleProtocol` |
| 只有 `prefer_simple_protocol=false` | 保持默认模式 |
| 只有 `default_query_exec_mode=<合法 v5 模式>` | 使用用户指定的原生 v5 模式 |
| 同时显式配置新旧两种参数 | 初始化时报出配置冲突，请用户保留一种；不静默覆盖 |
| 旧布尔参数或 v5 模式值非法 | 初始化时返回配置错误，不连接服务端 |

旧布尔参数沿用 v4 的 `strconv.ParseBool` 语义，不与 `PREFER_IPV4_STACK` 环境变量的精确字符串匹配规则混淆。应使用 pgconn/pgx 的连接解析能力识别参数及显式配置来源，支持 URI、键值字符串及原有连接配置来源，不用字符串正则粗暴改写 DSN。

v5 不再消费 `prefer_simple_protocol`，直接将旧 DSN 交给 v5 可能把它保留在 `RuntimeParams` 并发送给服务端。兼容处理必须删除该旧键，并验证它没有出现在启动参数中。还需盘点 `statement_cache_mode` 等其他 v4 专用参数的迁移，不能仅替换结构体字段就声称连接字符串完全兼容。

拟增加的配置入口测试矩阵：

- URI、键值两种形式，覆盖旧参数 true/false、v5 原生模式、缺省、非法值及新旧参数同时配置。
- 从真实 `[[instances]]` TOML，经 `cfg.LoadSingleConfig` 等实际加载入口、`Instance.Init` 和公共解析函数，断言最终 `ConnConfig.DefaultQueryExecMode` 及运行时参数。
- 验证 `IsPgBouncer` 没有被手动赋值；不通过测试专用分支制造不存在的生产入口。
- 真实 PostgreSQL/PgBouncer 集成同样使用用户配置文件；同时验证不会收到未知参数 `prefer_simple_protocol`，并按实际支持的 pool mode 验证采集。
- SQL 注入回归中可以显式传入攻击参数以覆盖驱动漏洞，但必须与“生产用户配置如何选择协议模式”的测试分别说明，不能相互替代。

评审核对时，已通过临时探针实际加载 TOML 并调用现有 `Instance.Init`：URI 和键值形式的旧参数均被 v4 解析为 simple protocol，`IsPgBouncer` 均保持 false；仅设置 `prepared_statements=false` 时仍被初始化覆盖为 true。探针未连接数据库，临时源码已清理；这只是当前入口证据，不是未来 v5 兼容处理已实现或集成测试已通过的证明。

### 4.3 功能回归

- PostgreSQL 直连与 PgBouncer，覆盖实际支持的池化模式。
- URI、键值连接字符串、Unix socket 和 TLS 连接。
- 默认指标、自定义 SQL、查询超时和错误返回。
- `database/sql` 扫描的整数、小数、时间、字符串与 NULL，确认指标数值和标签未变化。
- #50 的畸形 DataRow 回归与 #69 的实际驱动参数替换回归。

### 4.4 PostgreSQL 实施与验证结果（2026-10-29）

已完成：

- PostgreSQL 插件使用 `github.com/jackc/pgx/v5 v5.9.2`，以明确的 `pgx/v5` 驱动名打开 `database/sql` 连接。
- `connection.go` 由真实 `Instance.Init` 调用，消费旧 simple/cache 参数、映射查询模式，并拒绝新旧模式冲突。兼容 URI、键值 DSN、service file、Unix socket 文件路径；更新示例配置及插件 README。
- `go mod tidy` 移除了旧 `pgx/v4`、`pgconn`、`pgproto3/v2`、`pgtype` 等驱动依赖；模块图、vendor 包依赖及完整 Linux 二进制均核对为 v5。已重新生成本地 vendor；该目录按仓库现有规则忽略，不加入提交。

新增回归分为三个文件：

| 文件 | 验证内容 |
| --- | --- |
| `inputs/postgresql/connection_test.go` | 从 TOML 加载并初始化；旧布尔语义、原生模式、非法值/冲突、缓存参数、service file、socket 路径，以及旧参数不进入 RuntimeParams |
| `inputs/postgresql/security_test.go` | DataRow 负长度、NULL、空值、截断与越界；模拟恶意服务端，通过实际注册驱动验证返回 DataRow 解码错误且不 panic，同时检查真实启动报文不含旧客户端参数 |
| `inputs/postgresql/integration_test.go` | 实际连接后的驱动模式、SQL 类型扫描、绑定参数、默认及自定义指标、SQL 错误/超时；simple protocol 中执行 dollar-quoted SQL 与恶意绑定参数并检查临时 canary 表 |

验证结果：

- `GOTOOLCHAIN=go1.25.10 go test -race ./inputs/postgresql`、`GOTOOLCHAIN=go1.25.10 go vet ./inputs/postgresql` 通过；Go 1.26.8 下的包测试及 race/vet 也通过。
- 默认完整构建通过：Linux amd64、Linux arm64、Windows amd64。
- 在授权测试机创建独立、可销毁的容器及网络，使用 PostgreSQL **16.14 / 18.4**、PgBouncer **1.25.2**（`pool_mode=transaction`、`max_prepared_statements=0`），并覆盖 `verify-full` TLS 及 Unix socket。
- 以上 5 个端点分别验证默认、旧 simple、新 simple、旧 describe 缓存、禁用缓存，25 组连接回归通过。带绑定参数的用例覆盖 PostgreSQL 的全部模式及 PgBouncer 兼容模式；PgBouncer 默认模式仅验证采集器当前不带绑定参数的查询，不据此承诺命名预备语句兼容。
- SQL 注入回归通过真实 TOML 入口选择 simple protocol，未设置 `IsPgBouncer=true`。验证 dollar-quoted 字面量保持原样、恶意参数保持为数据、canary 表仍可查询；临时表在事务结束后删除。
- 测试使用的容器、匿名卷、网络、证书、socket 和测试二进制均在结束后清理。

真实连接测试的环境变量和执行方式见 `inputs/postgresql/README.md`；未配置测试端点时会明确跳过集成部分。

**扫描范围与限制**：已运行 `govulncheck ./inputs/postgresql` 及 `govulncheck -scan module -json`，当前模块扫描没有 jackc 驱动链的受影响 finding，#69 对应的 GO-2026-5004 未命中，#50 的旧模块路径已移除。但扫描整体并非全绿：本地构建工具链 Go 1.26.8 的标准库存在其他告警，模块级扫描也报告现有 Docker、`x/net`、`x/crypto` 告警。这些结果不能当作 PostgreSQL 修复失败，也不能被描述为“仓库全部漏洞已修复”；需在对应依赖或工具链批次继续处理。

单独运行 errcheck 时仍报告 `postgresql.go` 原有的 7 处未检查 Close 返回值，均不在本次修改行；新增生产代码未出现 errcheck 报错。尚未运行远程 PR CI。

### 4.5 评审补充：旧读取缓冲区参数兼容

`min_read_buffer_size` 是 v4 连接层的客户端选项，v5 不再消费它。兼容层现校验并移除该键，接受 `0..2147483647` 的十进制整数；非法、空、负数和溢出值在初始化时报错。v5 自行管理缓冲区，旧数值不再应用，但原有合法 `address`（键值/URI）及 service file 无需改写，参数不会进入 PostgreSQL 启动报文。

先新增回归复现参数泄漏及非法值未被拒绝，再补实现。覆盖实际 TOML/Init、service file 与显式覆盖、默认/旧 simple/原生查询模式、原始连接串保持不变，以及真实启动报文。授权测试机重新运行 PostgreSQL 16.14/18.4、PgBouncer 1.25.2、`verify-full` TLS、Unix socket 的扩展矩阵，33 项实际执行通过，2 项明确跳过；临时资源已清理。

扩展矩阵发现第 4.4 节原测试的一个限制：PgBouncer `pool_mode=transaction` 且 `max_prepared_statements=0` 时，多次创建缓存模式客户端可能出现 SQLSTATE 42P05（预备语句已存在）。原先单次默认模式通过不能证明该部署组合兼容。当前两项默认命名缓存用例明确跳过，simple protocol、描述缓存、无缓存模式实际执行；旧 `min_read_buffer_size` 与 `prefer_simple_protocol=true` 的组合也已通过。生产环境仍应按已有协议参数选择兼容模式，不自动更改用户查询模式来掩盖该限制。

## 5. 第二批实施细节：独立 Docker SDK 迁移

### 5.1 实施前置条件：完整依赖闭包和 Prometheus 版本路线

在调整直接使用 SDK 的三个目录前，先完成以下工作并作为第二批 PR 的准入条件：

1. 对默认完整构建保存旧 Docker 模块的全部直接/传递引入路径，至少包括第 2 节的 Prometheus 服务发现链；额外检查发布使用的构建标签组合。
2. 核对模块模式与 vendor 模式的依赖结果，不能只搜索不含 vendor 的业务源码。
3. 选定并核验能够消除 Prometheus 旧 SDK 引入路径的固定版本或可复现补丁，再安排直接目录迁移。
4. 在隔离候选构建中确认 Prometheus 服务发现可使用最终选中的 Moby client/API 版本；候选版本在 Go 模块最小版本选择（MVS）后可能变化，不能假定两个 v0.x SDK 版本完全兼容。

当前已确认的原始导入关系可通过下列只读命令复核：

```sh
go list -mod=vendor -f '{{.ImportPath}}: {{join .Imports " "}}' \
  ./prometheus \
  github.com/prometheus/prometheus/discovery/install \
  github.com/prometheus/prometheus/discovery/moby
```

**主方案：将 Prometheus 从 v0.311.3 升至固定候选 v0.312.0。** 已核对该 tag 的 `go.mod` 和 `discovery/moby/docker.go`：Go 要求为 1.25.0，Docker 服务发现已导入 `github.com/moby/moby/client` 及 `github.com/moby/moby/api`，其模块原始版本分别为 v0.4.1、v1.54.2。该结论是版本/源码核验，尚未完成与本项目及最终 SDK 版本的组合构建和运行验证。

本方案不选择未固定的 Prometheus `main` 或直接更新到 `latest`。v0.312.0 是明确的最小迁移候选，不是免除其他安全公告检查的最终发布承诺。需同时适配本项目使用的 scrape、remote、TSDB/agent 和配置接口，并验证 Prometheus 服务发现的实际可用性。

**备选：若整体升级不兼容，维护基于 v0.311.3 的可复现回移补丁。** 上游迁移参考提交为 `39672984b5cc9f7fb61dfcf6df5693b1137ca6cb`，以及后续 API 调整 `ab63eeb0d228a3cf70c15b8181da05831db93a76`。备选需在独立依赖仓库固定不可变提交，以模块 `replace` 引用经过验证的补丁版本，并记录基线、补丁来源、适配差异、校验值和再生成步骤。补丁至少覆盖 `discovery/moby` 全部 SDK 使用、相关测试和模块依赖调整；不能只改本地 vendor 中的几个文件。

主方案无法通过组合验证时，必须先完成并固定备选补丁，再继续把“移除旧模块”作为交付目标；不能留下 Prometheus 旧路径却声明安全依赖清理已完成。保留 `docker_sd_configs`、`dockerswarm_sd_configs` 的注册和功能，不以删除 `discovery/install` 或关闭 Prometheus 功能绕过问题。

参考：[Prometheus v0.312.0 模块配置](https://github.com/prometheus/prometheus/blob/v0.312.0/go.mod)、[Docker 服务发现实现](https://github.com/prometheus/prometheus/blob/v0.312.0/discovery/moby/docker.go)、[上游迁移提交](https://github.com/prometheus/prometheus/commit/39672984b5cc9f7fb61dfcf6df5693b1137ca6cb)、[后续 API 调整](https://github.com/prometheus/prometheus/commit/ab63eeb0d228a3cf70c15b8181da05831db93a76)。

### 5.2 SDK 候选版本与完整修改范围

核对时可用且相互匹配的候选版本：

```text
github.com/moby/moby/client v0.6.2
github.com/moby/moby/api    v1.56.1
```

client v0.6.2 的模块依赖明确包含 api v1.56.1，两者 Go 要求为 1.24。实施时应再次核对安全公告和版本兼容性，并固定经过验证的版本，不无条件使用 `latest`。

前置闭包检查通过后，修改以下直接使用目录的导入、接口封装、返回值和测试 mock，并同步实施选定的 Prometheus 路线：

- `inputs/docker/`
- `logs/util/docker/`
- `inputs/keepalived/types/container/`
- `prometheus/` 及固定 Prometheus 模块中的 `discovery/moby/`，后者通过上游版本升级或固定补丁提供。

需适配 Container/List/Inspect/Stats、镜像 Inspect、事件流、日志流、Swarm 查询及 exec 等 API。不能只对 `go.mod` 加模块路径 `replace` 而保留旧版调用签名。

迁移后再次检查与前置基线相同范围的依赖闭包，确认直接路径和 Prometheus 服务发现路径均已消除，并检查是否还有其他引入来源。Categraf 仅需要客户端和 API 类型，不为消除告警而引入 Moby v2 的完整 daemon 模块。

### 5.3 CPU Stats 行为兼容

当前 CPU 使用率基于 `PreCPUStats` 与 `CPUStats` 的差值计算。新版 SDK 的非流式 Stats 默认不请求前一次样本，直接沿用默认值会改变指标含义。

对于当前需要两份样本的调用，显式设置：

```go
client.ContainerStatsOptions{
    Stream:                false,
    IncludePreviousSample: true,
}
```

核对指标采集和日志工具中的所有 Stats 调用，验证 `PreRead`、`PreCPUStats`、Linux/Windows CPU 使用率和响应体关闭行为。不能以“编译通过”替代指标一致性验证。

参考：[新版 Stats 选项与默认行为](https://raw.githubusercontent.com/moby/moby/client/v0.6.2/client/container_stats.go)。

### 5.4 Docker API 版本兼容

候选 client v0.6.2 的最低 API 版本为 `1.40`，需将这个限制与项目实际支持范围核对。保持自动 API 协商，并确认 `DOCKER_HOST`、TLS 和显式 `DOCKER_API_VERSION` 的行为。

测试应覆盖项目声明支持的旧 daemon、28.x 和 29.x；若需要支持 API 1.40 之前的 daemon，应在选定候选版本前解决兼容性，不静默降低现有支持范围。

参考：[SDK 版本限制与协商逻辑](https://raw.githubusercontent.com/moby/moby/client/v0.6.2/client/client.go)、[Docker 官方 SDK 文档](https://docs.docker.com/reference/api/engine/sdk/)。

### 5.5 功能回归

- 容器列表、状态与 Inspect，包含退出、删除和查询失败场景。
- CPU、内存、网络、磁盘指标及相应标签一致性。
- Swarm 的 Service、Task、Node 查询。
- 日志读取、事件订阅、重连与取消。
- 镜像名解析、镜像/容器不存在时的错误判断。
- Keepalived 的 exec 创建、attach、输出读取和资源释放。
- Unix socket、支持平台的 Windows named pipe、远程 TLS 连接。
- Client、响应体、事件和日志流在退出/取消时正确关闭。
- Prometheus `docker_sd_configs` 和 `dockerswarm_sd_configs` 经真实 YAML 配置入口成功注册、解析并创建发现器。
- Docker 服务发现的容器/网络过滤、目标地址、端口、host networking 及 `__meta_docker_*` 标签与迁移前一致。
- Docker Swarm 的 nodes、services、tasks 各角色发现、目标地址及元标签一致。
- 服务发现的 Unix socket/TLS、API 协商、刷新、超时、错误与取消正常；除第三方包测试外，还需经过本项目的注册和配置入口回归。
- 默认完整 Prometheus 抓取、remote write 和 TSDB/agent 功能通过兼容性检查；`no_prometheus` 等裁剪构建只作为额外验证，不替代完整构建。

### 5.6 Docker SDK 实施与验证结果（2026-10-29）

**最终依赖组合**：`github.com/moby/moby/client v0.6.2`、`github.com/moby/moby/api v1.56.1`、`github.com/prometheus/prometheus v0.312.0`。实施前已在临时模块中通过这三个固定版本的发现器组合编译；最终采用上游版本升级，无须维护 Prometheus fork 或修改 vendor 源码。

实施范围：

- `inputs/docker`：迁移接口适配器、列表/Inspect/Swarm 返回值和新选项；Stats 显式设置 `IncludePreviousSample: true`；从 Info 保存 daemon OS 类型，保持 Linux/Windows CPU、内存、网络、块设备和状态指标及标签。
- `logs/util/docker`：迁移镜像、容器、网络端口、事件和日志接口，保留镜像缓存与 not-found 错误分类；关闭 Stats 响应体，处理有限事件流 EOF、重连、启动阻塞、退避和背压时的取消。
- Keepalived：迁移 exec create/attach/kill，释放 hijacked connection，并按非 TTY 协议解复用 stdout/stderr。
- Prometheus：通过上游 `discovery/install -> discovery/moby` 保留 Docker/Swarm 注册，新增完整 YAML 入口测试。存储兼容回归同时补全了 `readyStorage.StartTime` 的 agent 分支，以及 agent WAL 的 remote write 通知。

**额外依赖闭包**：完成上述迁移后，模块图仍有 `google/gnxi -> docker/docker v24.0.7`，以及旧完整 Moby 模块要求。Categraf 仅使用 `gnxi/utils/xpath`，上游较新提交也保留这些 daemon 依赖。因此，将原先精确版本 `ef18504847b0` 的两个纯解析文件保留在 `internal/gnmipath`，包含来源、原始版权及 Apache-2.0 许可证；仅修改包名、来源注释和两处等价的 `ReplaceAll` 调用以通过 lint。新增解析和订阅路径兼容测试后，移除 `google/gnxi` 模块。没有通过删除 gNMI 或服务发现功能绕过依赖。

最终模块图、`go.mod`/`go.sum`、vendor 模式和模块模式的默认完整构建依赖中，均不再包含旧 `github.com/docker/docker` 或完整 `github.com/moby/moby` 模块。两种模式的包依赖清单一致，完整 Linux 二进制记录的新 SDK、Prometheus 和 PostgreSQL 版本均符合预期；本地 vendor 已重新生成，继续遵循仓库忽略规则。

测试与实际验证：

| 层次 | 结果 |
| --- | --- |
| 配置与请求 | TOML 初始化；ENV 主机/API 覆盖；API 1.40、1.51、1.54 协商；明确 Ping 拒绝过低协商版本；Unix socket 和验证 CA 的 TLS |
| 指标与类型 | 两次 CPU 样本、Linux/Windows CPU 比例、三种 memory cache 形式、CPU/内存/网络/块设备指标和环境/标签；Swarm 服务、任务和节点返回值 |
| 日志与资源 | 镜像名缓存、Inspect size/缺失/非法响应、卷过滤、日志流；Stats 正常及解码错误时关闭 Body；有限事件 EOF、重连、错误退避、背压及阻塞建连时取消 |
| Keepalived exec | 新建/attach/kill 请求、stdout/stderr 解复用、创建/attach 错误，以及真实 shell exec；不假定两个输出流的到达顺序 |
| 项目服务发现 | 真实 Prometheus YAML 注册/创建发现器；Docker 过滤参数、地址/端口、host networking、元标签；Swarm nodes/services/tasks 的地址及对象/网络元标签；Unix/TLS、失败重试和取消 |
| 上游服务发现 | 在本项目最终依赖组合下执行 `go test -mod=readonly github.com/prometheus/prometheus/discovery/moby`，通过 |
| Prometheus 存储 | 真实 HTTP 抓取 → agent/TSDB WAL → remote write，解码接收端 protobuf 并核对样本值，通过 |
| 真实 Engine | 20.10.24（API 1.41）、28.4.0（API 1.51）、29.5.2（API 1.54）：指标、日志/事件、exec、Docker 发现全部通过；20.10/29.5 在独立嵌套 daemon 中验证真实 Swarm 三角色发现 |
| 静态检查与构建 | Go 1.25.10 的相关包 race/vet 通过；golangci-lint 2.12.2 按完整新增补丁（包含未跟踪 Go 文件）与 `--tests=false` 检查迁移范围，0 issues；Linux/Windows amd64/arm64 完整构建与 Linux 裁剪构建通过 |

真实 Engine 测试通过授权测试机执行，保留了共享 daemon 的原有配置。测试结束后移除临时容器、匿名卷、Swarm daemon、网络、镜像及文件。最低 API 1.40 由协议夹具覆盖，未运行真实 Docker 19.03 daemon；Windows named pipe 仅完成交叉编译验证，未做 Windows 主机运行测试。

可重复执行的环境变量、用例及清理命令见 `inputs/docker/README.md`。

**安全扫描边界**：模块级扫描未再命中本文件 6 条目标告警；源码及构建二进制也已扫描。仓库扫描整体仍有其他结果，不能将本批验收等同于“全部依赖无漏洞”：

- 本机 Go 1.26.8 / 兼容性构建 Go 1.25.10 的标准库，以及现有 `x/net`、`x/crypto` 有独立告警，需另行安排工具链/依赖升级。
- Prometheus v0.312.0 的 Outscale 提供方经 `osc-sdk-go/v2 v2.34.0` 新增 `aws-sdk-go v1.55.8`。模块扫描命中 S3 Crypto 的 GO-2022-0635 / GO-2022-0646（CVE-2020-8912 / CVE-2020-8911），均无旧模块原地修复版本；默认完整构建未导入 `service/s3/s3crypto`，源码扫描未给出相应可达调用。上游 Outscale v2.35.0 仍要求同一旧 AWS SDK，不能以无效升级宣称已消除。这两项作为新传递依赖的模块告警单独跟踪，不计入本次 6 条已迁移目标。

远程 PR CI 与 GitHub 告警关闭状态尚待确认。Docker 主机补丁状态仍由第 6 节的独立部署流程确认。

### 5.7 Copilot 评审：部署验证与剩余风险接受

对应 [PR #1509](https://github.com/flashcatcloud/categraf/pull/1509)。技术验证与负责人接受是两件独立事项；当前没有将维护者验证或安全风险接受标记为完成，也没有关闭/忽略 AWS 告警。

| 部署范围 | 验证证据与边界 |
| --- | --- |
| Linux Docker 20.10.24 / 28.4.0 / 29.5.2 | 第 5.6 节真实 daemon 回归通过；20.10/29.5 包含真实 Swarm 三角色发现 |
| Docker API 1.40 | 协议夹具通过，未运行 Docker 19.03 daemon；新 SDK 支持下限需纳入发布说明 |
| PostgreSQL 16.14 / 18.4、PgBouncer 1.25.2 | 第 4.5 节扩展矩阵通过；不支持的事务池命名缓存组合明确跳过 |
| PostgreSQL 14/15 与更老部署 | pgx v5.9.2 上游声明支持 PostgreSQL 14+；本次没有实际运行 14/15，更老版本没有兼容性保证，维护者需确认项目支持范围 |
| Windows amd64 named pipe | 新增 `TestMobyNamedPipe`：真实 Win32 pipe listener，经生产 ENV/显式端点构造器覆盖协商、Info/List/Inspect/Stats、前次 CPU 样本和请求超时；本地完成 Windows 测试二进制交叉编译；新增 Windows runner CI，实际结果待确认 |
| Windows daemon / Windows arm64 | 前次完整交叉编译通过；上述 pipe 夹具不等同于真实 Windows Docker daemon 或 arm64 主机端到端验证，仍需维护者确认发布范围 |

新增 Windows CI 不需要安装 Docker 或访问宿主机 daemon，只创建当前测试用户自己的临时命名管道，并关闭 listener/HTTP server/client。它用于验证原生传输路径，不能被描述为所有 Windows 部署均已验证。

两条 AWS 告警的复核证据：

- 引入链固定为 `prometheus v0.312.0 -> discovery/outscale -> osc-sdk-go/v2 v2.34.0 -> aws-sdk-go v1.55.8`。
- Outscale 调用 `aws/credentials.NewStaticCredentials` 与 `aws/signer/v4.NewSigner` 进行请求签名。
- [GO-2022-0635](https://pkg.go.dev/vuln/GO-2022-0635) / [GO-2022-0646](https://pkg.go.dev/vuln/GO-2022-0646) 的受影响包均为 `service/s3/s3crypto`，符号为 `NewEncryptionClient` / `NewDecryptionClient`，旧模块没有已知修复版本。
- 本次重新核对默认 Linux 和 Windows amd64 完整构建的 `go list -deps`，均未导入该 S3 Crypto 包；重新执行源码 `govulncheck -json ./prometheus`，两条 finding 仅为模块级，未出现该包/符号调用链。这是当前版本的不可达性证据，不是风险接受决定。
- 依赖升级或新增 S3 Crypto 调用会使当前证据失效；后续移除旧 AWS SDK 需要继续迁移 Outscale 客户端，不能通过隐藏模块扫描结果替代。

待指定维护者确认实际支持的部署范围，待指定安全负责人决定是否接受当前两项剩余模块告警。确认记录应引用对应提交和适用发布范围，当前记录均为“待确认”；助手不代替负责人签署，也不从 Copilot 的建议或 CI 通过推断已经接受。

## 6. Docker daemon 部署侧说明

针对本文件四项 Docker 告警，daemon 修复下限如下：

| 告警 | 对应 Engine 修复下限 |
| --- | --- |
| #66 | 29.3.1 |
| #75、#77、#76 | 29.5.1 |

这些是针对相应漏洞的修复下限，不是建议部署的当前版本。实际应选择发行方仍支持、包含这些修复的更新补丁版本，或确认发行方已回移对应补丁。29.5.1 曾引入特定 bind mount 场景下的 docker cp 回归，29.5.2 已修复，因此部署选型不能只满足漏洞版本下限。

Categraf 代码迁移与宿主机 Docker daemon 更新分别安排，不能用 SDK 发布替代主机补丁。

参考：[Engine 29 发行说明](https://docs.docker.com/engine/release-notes/29/)。

## 7. 统一验证与完成标准

### 7.1 代码和功能检查

- 保留现有测试并增加上述安全回归、协议集成和 SDK 兼容性测试。
- 执行相关包测试、适用的 race 检查、go vet 和 CI 中的 golangci-lint。
- 对共享测试辅助文件也执行 errcheck；非 `_test.go` 文件即使服务于测试，也可能在 `--tests=false` 时被检查。
- 按项目支持范围完成 Linux/Windows、amd64/arm64 构建，验证完整版本及相关构建标签组合。

### 7.2 安全与依赖检查

- 对照迁移前后的 `go.mod`、`go.sum` 和依赖图，确认旧模块及引入来源已经移除。
- Docker 完整依赖闭包盘点与 Prometheus 固定迁移路线是实施前置门槛，交付时按同一范围再次验收。默认完整构建的 `go list -deps` 不再包含 `github.com/docker/docker` 包；模块图也不再由 Prometheus 的模块要求保留该旧依赖，不能仅验证三个直接目录或裁剪二进制。
- 核对模块模式和 vendor 模式的构建结果一致。
- 使用 `govulncheck` 检查源码调用路径及实际发布二进制，重点复核本文件 6 条告警；若发现其他告警，单独记录，不混同为本次已修复。
- 未发现可达路径的 Docker 告警，保留调用图和使用范围证据；不将“模块被扫描命中”直接等同于“存在可利用服务端漏洞”。
- 合入默认分支后等待 GitHub 重新分析依赖，逐条确认这 6 条告警的最终状态。若仍有告警，检查残留依赖和分析结果，不仅以本地构建通过宣布完成。

### 7.3 交付与回滚

- PostgreSQL 与 Docker SDK 分别提交 commit，降低审查和回滚范围；本次使用同一 `fix/` 分支推送。
- PR 描述中列出对应告警编号、版本变化、兼容性变化、测试环境和结果。
- 若必须回滚到旧依赖，应同时恢复对应安全风险记录，不能继续声称漏洞已修复。
- Docker 主机侧修复以独立维护记录确认，不能以 Categraf 发布替代。

## 8. 状态说明

两批代码迁移和验证结果分别记录在第 4.4、5.6 节，均已完成。评审指出的 PostgreSQL 真实配置入口和 Prometheus Docker/Swarm 传递依赖已纳入实现与回归；实施时额外清理了 gNMI XPath 带入的旧 Docker 模块要求。剩余事项包括 PR CI、GitHub 依赖重新分析、单独记录的其他扫描告警，以及部署侧 daemon 补丁确认。

当前代码、未来补丁和实际上线版本可能不同；实施时应以目标分支的依赖图与最新上游公告再次确认。
