# gNMI 本地修复使用与验证说明

更新日期：2026-10-30。本文记录本地修复分支的使用方式、用户设备现场验证结果及待办事项；不代表所有厂商设备均已验证。

## 当前行为与配置

已合入官方 upstream/main 的 `030dcba`，上游合并提交为 `da05283`，包括使用 `internal/gnmipath` 替代旧 XPath 依赖。本地保留 inline origin 归一化、订阅与别名匹配优先级、配置错误同步返回及 IEEE Float32 转换失败跳过等修复。

`tag_preload_timeout` 配置及其预加载、超时回放逻辑已移除。升级时请从现有配置删除该参数；不要依赖旧参数产生等待效果。当前没有默认 5 秒等待，也没有“必要标签齐全才输出”的保证。

当前处理方式：在同一 Notification 中先处理标签更新，再输出数值；跨 Notification 按接收顺序处理。晚到的标签只影响后续指标，已经输出的指标不会补写。按设备地址保留标签缓存并在重连时复用，但缓存存在不代表标签仍然新鲜。上游本来就会在普通重连循环内复用标签，本地进一步将存储提升至 Instance、按地址管理并加锁。

实例配置示例（将地址、认证和编码替换为设备实际设置）：

```toml
interval = "1s"

[[instances]]
addresses = ["192.0.2.1:50051"]
username = "YOUR_USERNAME"
password = "YOUR_PASSWORD"
encoding = "json"
agent_host_tag = "ident"
redial = "10s"

[[instances.tag_subscription]]
name = "hostname"
path = "/rg-system:system/hostname"
subscription_mode = "sample"
sample_interval = "0s"
suppress_redundant = false
match = "unconditional"
disable_concatenation = true

[[instances.subscription]]
name = "gnmi_system_cpu_util"
origin = "openconfig-platform"
path = "/components/component/cpu/utilization/state"
subscription_mode = "sample"
sample_interval = "0s"
suppress_redundant = false

[[instances.subscription]]
name = "gnmi_system_memory"
origin = "openconfig-platform"
path = "/components/component/state/memory"
subscription_mode = "sample"
sample_interval = "0s"
suppress_redundant = false
```

说明：

- `sample_interval = "0s"` 透传设备，实际周期由设备决定。本次设备现场要求零间隔，不应推广为所有设备的要求。
- CPU 容器保留字段后缀，输出 `gnmi_system_cpu_util_instant`、`gnmi_system_cpu_util_avg`；内存容器输出 `gnmi_system_memory_available`、`gnmi_system_memory_utilized`。单位应依据设备模型确认，插件不自动换算。
- hostname 使用 `unconditional`，贴给同设备地址的指标。标签订阅本身不输出普通指标；其 `disable_concatenation=true` 保留标签名 `hostname`，不决定 CPU 指标命名。
- 路径可使用 `/openconfig-platform:components/...` 的 inline 形式，也可使用独立 `origin` 与 `/components/...`。设备是否接受对应请求形式需现场验证。
- 不要写成 `/openconfig-platform:/components/...`：模块名与首节点之间多出的斜杠会形成非法节点。初始化报错中的路径可能来自其他订阅，应检查所有加载配置。
- 省略或清空 `subscription_mode` 默认使用 SAMPLE。`conversion = "ieeefloat32"` 仅用于对应编码数据，普通整数、浮点值不应开启。

## 用户设备现场验证记录

以下结果来自用户反馈。预加载方案已撤回，不能列为“必要标签同步通过”。

| 序号 | 验证项目 | 结果与判定 |
| --- | --- | --- |
| 1 | 基线：标签识别与正常指标 | 通过。正常收到标签后 CPU、内存指标携带 hostname，标签不单独输出为指标；不代表首批标签完整性通过。 |
| 2 | 默认、关闭、自定义预加载时长 | 方案未满足需求，现已移除。首次仍可能输出不带标签的指标，必要标签同步列为暂定。 |
| 3 | inline origin 路径匹配 | 通过。合法模块前缀路径能匹配标签及数值订阅。 |
| 4 | 固定指标名与拼接控制 | 通过。叶子订阅开启 `disable_concatenation=true` 后不再重复追加叶子名；容器订阅保留正常后缀。 |
| 5 | 缺省订阅模式与零采样间隔 | 通过。缺省模式使用 SAMPLE，零间隔允许透传。 |
| 6 | 错误配置启动时报错 | 通过。非法订阅模式、编码、标签匹配策略等在初始化阶段明确报错。 |
| 7 | 转换失败跳过坏字段 | 通过。错误 IEEE Float32 数据记录错误并跳过，不输出误导性零值；其他正常字段继续处理。 |
| 8 | 冗余抑制警告 | 通过。`suppress_redundant=true` 且缺少正数 heartbeat 时警告，不拒绝订阅。 |

现场证据：连接于 01:22:21 建立，旧预加载在 01:22:26 超时并输出无 hostname 的内存指标；01:22:34 才记录 hostname，之后 CPU 指标带标签。因此固定等待时长不能保证必要标签完整；已输出指标也不会被追溯补齐，可能形成不同的时间序列。

设备测试命令：

```bash
/data/categraf/categraf --test --debug --inputs gnmi
```

每次只改一组参数并重启，至少观察 30 秒，完成后恢复基线。原始消息追踪可在实例配置开启 `dump_responses=true`；`--debug` 本身不等同于开启协议消息追踪。

开发测试命令（项目根目录）：

```bash
go test ./inputs/gnmi ./internal/gnmipath -count=1 -v
```

自动化补充覆盖 origin 强弱匹配、转换规则隔离、Update.Path 独立 origin 继承、同地址缓存复用及跨地址隔离、初始化失败后停止、上游 XPath 路径解析等。现场通过项不代表这些全部经过真实设备验证。

本轮发布按用户要求仅同步实现与说明文档，新增和修改的回归测试保留在原开发工作区，不随 PR #1508 同步。因此本文提到的 `origin_consistency_test.go`、`tag_origin_test.go` 等新增测试入口属于本地验证记录；PR 分支保留原有测试和官方上游基线测试，不能据此认为远程包含全部新增用例。

## 暂定：必要标签齐全后输出指标

状态：待设计、未实现。本次只移除超时预加载，不实现新的同步机制。

目标：对明确声明为必要的标签（例如设备 hostname、接口 ifname），指标必须具备对应标签后才输出。不能将所有标签默认视为必要，某些标签只适用于特定设备或接口。

后续方案方向：

1. 声明必要标签及其适用的数值订阅，区分设备级和接口级作用域。
2. 接收数值时按匹配作用域检查必要标签；未就绪则进入有界队列，标签到达后立即释放相关指标，保留原始设备时间戳。
3. 不将 `sync_response` 等同于必要标签齐全。不同接口分别判断，单个接口缺标签不能阻塞其他接口。
4. 明确等待时限、缓存条数/字节数上限、断线及停止时的处理策略。若要求“绝不输出缺必要标签的指标”，达到上限时应丢弃并告警，不能放行无标签指标。
5. 明确标签有效期、重连旧缓存是否可用、标签变更/删除及历史缓存数据使用哪一版标签，避免错误回填。
6. 提供缺失标签、缓存量、丢弃数量等可观测信息，并覆盖乱序、永不返回标签、同步响应先到、多设备与多接口场景。

上述为建议，必要标签配置格式、时限与丢弃策略尚未定稿，当前不可配置使用。

## 第二阶段：标签来源与缓存隔离

状态：代码已实现，等待更新二进制后进行设备现场验证。必要标签齐全后才输出仍未实现。

原问题：`equalsPathNoKeys()` 只比较路径段，handler 采用首个匹配项；标签缓存又不保留订阅及来源身份，造成同路径不同来源误识别、通用订阅抢占、同名标签静默覆盖。旧函数已移除，标签识别改用专门的来源选择器。

### 当前匹配规则

先要求完整路径段相同，忽略路径键。内部保留独立 origin 的来源标记与首节点 inline 模块名，不增加用户配置项，不改变发给设备的请求路径。

标签订阅未单独配置 origin 时，内部来源路径继承实例全局 origin，包括 prefix 为空的情况；不会因为省略 prefix 就退化为通用订阅。

| 配置与设备来源 | 处理 |
| --- | --- |
| 双方独立 origin 相同，且已知模块无冲突 | 精确匹配，优先级 3 |
| 双方独立 origin 不同 | 此订阅不匹配，即使模块名相同 |
| 配置独立 origin，设备仅 inline 模块，双方模块相同 | 兼容匹配，优先级 2 |
| 配置独立 origin，设备没有来源信息且无法验证模块 | 此订阅不匹配并限频诊断 |
| 配置仅 inline 模块，与设备模块相同；或设备独立 origin 等于配置推断的 origin | 模块匹配，优先级 2；已知模块冲突时拒绝 |
| 配置没有 origin、也没有 inline 模块 | 通用回退，优先级 1 |

高优先级覆盖低优先级，不依赖配置顺序。例如配置 `origin="rfc7951"` 与 `/rg-system:system/hostname`，设备仅发送 `rg-system:system` 时仍可识别。

独立 origin 本身就是模块名时，也可匹配仅有 inline 模块的响应，例如 `origin="rg-system"`、`path="/system/hostname"` 可识别 `/rg-system:system/hostname`。标签、数值订阅选项及 alias 共用来源兼容规则；双方显式 origin 不同或已知 inline 模块冲突时均不匹配。数值响应完全没有来源信息时保留原有回退行为，标签仍要求可验证的来源。

评审修复追加的 `origin_matching_test.go` 随 PR 同步，覆盖上述表示方式兼容、跨模块转换隔离、强匹配优先及兼容 origin 下的指标后缀截取。此前提及但未同步的其他本地测试仍以原说明为准。

同等最高优先级有多个候选时不消费为标签，保留普通数值处理路径并限频告警；不会任意选第一项。初始化可识别的重复来源（包括相同独立 origin、相同路径且模块限定有重叠）直接报错。不同独立 origin 却共享 inline 模块的配置，在设备省略独立 origin 时可能形成运行时歧义，此时不会猜测来源。

### 缓存与标签应用

每个设备地址下按“配置订阅身份 + 实际来源身份”分桶，之后按 unconditional、name 或 elements 的作用域存储。写入和空值清除只影响同一桶；桶空时回收。第一节点 inline 模块名在 elements 键匹配中同步去除。

来源隔离不限制标签的应用模块：rg-system 的 hostname 仍可贴给 openconfig-platform 的 CPU/内存指标。`match` 的应用范围保持不变。

多个适用来源产生相同最终标签名时：同值合并，异值省略该标签并告警，其他标签继续附加。最终标签名按输出规则归一化后检测冲突，因此连字符、斜杠等转换为下划线后重名也能被识别。不再采用“最后到达者覆盖”。

标签来源不匹配/歧义诊断在每个连接 handler 中最多每分钟一条，缓存冲突诊断在每个设备缓存中最多每分钟一条；日志带设备地址，不打印冲突值。诊断限频不改变每次实际的匹配和冲突判定。

实际来源身份包括独立 origin 标记、origin 和已观察到的 inline 模块。设备变更来源表示方式可能建立新桶；未引入 TTL 或自动把不同来源桶视为同一来源，旧值保留和清理由现有更新语义决定。来源或标签值长期变化时应检查冲突日志。

### 回归与部署验证

自动化测试位于 `tag_origin_test.go`，结合已有测试覆盖匹配优先级与交换配置顺序、显式 origin 冲突、缺失来源、模块兼容、重复来源初始化拒绝、运行时歧义、三种 match 的缓存冲突/同值合并/单来源清除、通用订阅多个实际来源、inline 元素键，以及已有跨模块 hostname、重连和多设备隔离。

部署后先保持现有已验证配置，确认 CPU/内存正常携带已收到的 hostname；再单独验证来源冲突和重复订阅。出现 `ambiguous tag source`、`tag source mismatch` 或 `tag conflict` 时应核查配置和设备原始消息。当前无必要标签门控：冲突标签被省略或标签尚未收到时，指标仍可能输出。

## PR #1508：第一阶段执行记录

评审回复指出显式 origin 搭配 inline 模块路径导致标签失效，以及空 origin 订阅覆盖显式 origin 的选项。PR 的旧分支存在这两项；本地后续提交已修复原始场景，本阶段补齐剩余归一化不一致。

执行步骤：先新增失败测试，再最小修改 `pathInfo.normalize()`，最后运行 gNMI 与 XPath 包测试和静态检查。新增测试首先复现别名、更新、前缀继承、JSON 路径把 `rfc7951` 覆盖为模块名，以及端到端指标重复出现 `_speed_speed`。

本阶段只修复内部归一化；保留已有强匹配优先、无 origin 回退及同等强度下后配置者优先的行为。回归检查还覆盖交换通用订阅顺序、不同非空 origin 的选项隔离、IEEE Float32 正常转换，以及原始配置和发给设备的 inline 请求保持不变。

测试入口：`origin_consistency_test.go` 与既有 `origin_precedence_test.go`。第一阶段没有实现标签来源隔离；第二阶段的实现与边界见上文。必要标签等待仍未实现。
