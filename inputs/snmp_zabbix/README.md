## 一、插件概述
### 1.1 插件功能介绍
snmp_zabbix 是一款兼容Zabbix采集模板的SNMP 数据采集插件，其最大特色是能够直接使用 Zabbix 的 YAML 格式模板文件。这意味着您可以利用 Zabbix 丰富的模板生态系统，无需重新编写监控配置。
主要功能包括：

 - 完整的 SNMP 协议支持：支持 SNMPv1、v2c、v3 所有版本
 - Zabbix 模板兼容：直接使用 Zabbix 6.0+ 的 YAML 格式模板
 - 自动发现机制：自动发现网络接口、文件系统等资源并动态创建监控项
 - 强大的预处理：支持正则表达式、JavaScript、数值计算等多种数据预处理方式
 - 精细化的调度：支持item粒度调度采集任务
 - 健康检查与自动恢复：自动检测连接状态并重连

### 1.2 与snmp 插件的区别
|特性|SNMP_Zabbix 插件| SNMP 插件|
|--------|------|-------|
|配置方式|Zabbix 模板 + 简单配置|	手动配置每个 OID|
|自动发现|支持 LLD（低级别发现）|需手动配置|
|预处理	|支持 20+ 种预处理方式|基本数值转换|
|模板复用|可直接使用 Zabbix 模板库|需从零开始|
|配置复杂度|低（使用现成模板）|高（逐个配置）
|动态监控项|支持（通过发现规则）|不支持|

### 1.3 适用场景
从 Zabbix 迁移到 Categraf，希望复用现有监控模板
需要监控大量网络设备（交换机、路由器、防火墙等）
需要动态发现和监控变化的资源（如网络接口）
需要对采集数据进行复杂预处理
希望利用 Zabbix 社区丰富的模板资源

### 1.4 系统要求和依赖
- Categraf 版本：开源版 >= v0.4.24 企业版 >= v0.4.40
- 网络要求：能够访问目标 SNMP 设备的 UDP (默认161) 端口
- 目标设备：启用 SNMP 服务的网络设备或服务器
- 模板要求：Zabbix 6.0+ 及以上的YAML 格式模板（不支持旧版 XML 格式）

## 二、Zabbix 模板获取与管理
### 2.1 什么是 Zabbix 模板
Zabbix 模板是预定义的监控配置集合，包含了监控项、发现规则、触发器等配置。每个模板针对特定类型的设备或服务，如 "Cisco Switch"、"Linux SNMP" 等。

### 2.2 获取模板的方式
#### 2.2.1 从 Zabbix Web 界面导出
- 步骤 1：登录 Zabbix Web 界面
比如，https://your-zabbix-server/

- 步骤2: 导航到模板页面
数据采集(Data Collection) -> 模板(Templates)

- 步骤 3：选择要导出的模板
勾选需要导出的模板（可多选）,点击底部"导出(Export)"按钮
- 步骤 4：选择导出格式
重要：选择 "YAML" 格式（Zabbix 6.0+）,如果只有 XML 选项，说明 Zabbix 版本过低
- 步骤 5：保存文件
文件将自动下载，默认名称如：zbx_export_templates.yaml

#### 2.2.2 使用 Zabbix API 导出
方法 1：使用 curl 命令
```
# 1. 获取认证 token
# 7.0 以上版本, 请参考https://flashcat.cloud/blog/zabbix-to-flashcat/ 如何申请api token
# 以下接口均在7.2版本上验证， 7.0 以下版本接口和参数可能有些差异，请自行查询zabbix官网文档

# 2. 获取模板 ID
curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"method\": \"template.get\",
    \"params\": {
      \"output\": [\"templateid\", \"name\"],
      \"filter\": {
        \"name\": [\"Template Net Cisco IOS SNMPv2\"]
      }
    },
    \"id\": 2
  }" \
  http://your-zabbix-server/api_jsonrpc.php | jq .

# 3. 导出模板（获取到 templateid 后）
curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"method\": \"configuration.export\",
    \"params\": {
      \"format\": \"yaml\",
      \"options\": {
        \"templates\": [\"10255\"]
      }
    },
    \"id\": 3
  }" \
  http://your-zabbix-server/api_jsonrpc.php | jq -r .result > template_cisco.yaml
```

#### 2.2.3 使用官方/社区模板
 访问 Zabbix Git 仓库：
```bash
# 克隆整个模板仓库，从仓库中拷贝相关 yaml 文件
git clone git@github.com:zabbix/zabbix.git

# 或者直接下载特定模板
wget https://github.com/zabbix/zabbix/blob/master/templates/net/cisco/cisco_snmp/template_net_cisco_snmp.yaml
```
常用网络设备模板推荐：

|设备类型|模板名称|文件路径|
|--------|------|-------|
|Cisco 交换机| Cisco IOS by SNMP|templates/net/cisco/cisco_snmp/template_net_cisco_snmp.yaml|
|Huawei 交换机|Huawei VRP by SNMP|templates/net/huawei_snmp/template_net_huawei_snmp.yaml|
|HP 交换机|HP Enterprise Switch by SNMP|templates/net/hp_hpn_snmp/template_net_hp_hpn_snmp.yaml|
|Linux 服务器|Linux by SNMP|templates/os/linux_snmp_snmp/template_os_linux_snmp_snmp.yaml|
|Windows 服务器|Windows by SNMP|templates/os/windows_snmp/template_os_windows_snmp.yaml|
|通用网络设备|Network Generic Device by SNMP|templates/net/generic_snmp/template_net_generic_snmp.yaml|

这部分模板已经放到https://github.com/flashcatcloud/categraf/blob/master/conf/zbx_templates
### 2.3 模板文件格式转换（XML to YAML）

如果只有 XML 格式的模板，需要进行转换：

1. 把 XML 模板导入 Zabbix
2. 重新导出为 YAML 格式的模板


## 三、Zabbix 模板结构详解
### 3.1 模板基本结构
一个典型的 Zabbix YAML 模板结构如下：
```
zabbix_export:
  version: '7.0'
  date: '2024-01-15T10:00:00Z'
  templates:
    - template: Template Net Example Device
      name: Template Net Example Device
      description: Template for monitoring example network device
      groups:
        - name: Templates/Network devices
      items:           # 监控项
        - name: Interface {#IFNAME} incoming traffic
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.2.2.1.10.{#SNMPINDEX}
          key: net.if.in[{#IFNAME}]
          delay: 60s
          value_type: UNSIGNED
          units: bps
          preprocessing:
            - type: CHANGE_PER_SECOND
      discovery_rules:  # 发现规则
        - name: Network interfaces discovery
          type: SNMP_AGENT
          snmp_oid: discovery[{#IFNAME},.1.3.6.1.2.1.2.2.1.2]
          key: net.if.discovery
          delay: 1h
          filter:
            conditions:
              - macro: '{#IFNAME}'
                value: '{$NET.IF.NAME.MATCHES}'
                #value: '^(eth|bond|eno|ens)' # 直接这么写也行，上一行的写法只是为了用户宏的说明
                operator: MATCHES_REGEX
          item_prototypes:  # 项目原型
            - name: 'Interface {#IFNAME}: Bits received'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.2.2.1.10.{#SNMPINDEX}
              key: net.if.in[{#IFNAME}]
      macros:          # 用户宏
        - macro: '{$NET.IF.NAME.MATCHES}'
          value: '^(eth|bond|eno|ens)'
```
### 3.2 核心配置项说明
#### 3.2.1 Items（监控项）
监控项定义了要采集的具体指标：
```yaml
items:
  - name: CPU utilization         # 监控项名称（用于显示）
    type: SNMP_AGENT              # 类型（插件只处理 SNMP_AGENT）
    snmp_oid: .1.3.6.1.4.1.9.9.109.1.1.1.1.7.1  # SNMP OID
    key: system.cpu.util          # 唯一标识符
    value_type: FLOAT             # 数据类型
    units: '%'                    # 单位
    delay: 30s                    # 采集间隔
    preprocessing:                # 预处理步骤
      - type: MULTIPLIER
        parameters: ['0.01']      # 乘以 0.01 转换为百分比
```
Type 类型说明：
- SNMP_AGENT：SNMPv2c（插件支持）
- SNMPV1_AGENT：SNMPv1（插件支持）
- SNMPV3_AGENT：SNMPv3（插件支持）
- 其他类型（如 ZABBIX_AGENT、HTTP_AGENT）：插件忽略

Value_type 数据类型：
- FLOAT：浮点数
- CHAR：字符（作为标签处理）
- LOG：日志
- UNSIGNED：无符号整数
- TEXT：文本（作为标签处理）

Units 单位处理：
常见单位：
- B、KB、MB、GB：字节单位
- bps、Kbps、Mbps：速率单位
- %：百分比
- ms、s：时间单位

#### 3.2.2 Discovery Rules（发现规则）
发现规则用于动态发现资源并创建监控项：
```yaml
discovery_rules:
  - name: Network interfaces discovery
    type: SNMP_AGENT
    key: net.if.discovery
    delay: 1h                     # 发现间隔
    snmp_oid: discovery[{#IFNAME},.1.3.6.1.2.1.2.2.1.2,{#IFTYPE},.1.3.6.1.2.1.2.2.1.3]
    filter:
      evaltype: AND               # 过滤条件组合方式
      conditions:
        - macro: '{#IFNAME}'
          value: '^eth'
          operator: MATCHES_REGEX
        - macro: '{#IFTYPE}'
          value: '6'              # 以太网接口
          operator: EQUALS
    item_prototypes:              # 基于发现结果创建的监控项
      - name: 'Interface {#IFNAME}: Incoming traffic'
        type: SNMP_AGENT
        snmp_oid: .1.3.6.1.2.1.2.2.1.10.{#SNMPINDEX}
        key: net.if.in[{#IFNAME}]
```
Delay 采集间隔语法：
- 30s：30 秒
- 5m：5 分钟
- 1h：1 小时
- 1d：1 天
- 30：30 秒（纯数字默认为秒）
Filter 过滤器详解：

过滤器用于筛选发现的资源，只为符合条件的资源创建监控项。

操作符(operator)类型：

|操作符|说明|示例|
|--------|------|-------|
|EQUALS|完全匹配|value: `eth0`|
|NOT_EQUALS|不等于|value: `lo`|
|LIKE|包含|value: `eth`|
|NOT_LIKE|不包含|value: `docker`|
|MATCHES_REGEX|正则匹配	|value: `^(eth\|ens)`|
|NOT_MATCHES_REGEX|正则不匹配|value: `^lo`|

条件组合方式（evaltype）：

`AND`：所有条件都满足
`OR`：任一条件满足
`FORMULA`：自定义表达式

Filter 经典案例：

只监控物理网络接口：
```yaml
filter:
  evaltype: AND
  conditions:
    - macro: '{#IFNAME}'
      value: '^(eth|eno|ens|em)\d+$'
      operator: MATCHES_REGEX
    - macro: '{#IFADMINSTATUS}'
      value: '1'                  # 管理状态为 UP
      operator: EQUALS
```

排除虚拟接口和环回接口：
```yaml
filter:
  evaltype: AND
  conditions:
    - macro: '{#IFNAME}'
      value: '^(lo|docker|virbr|veth)'
      operator: NOT_MATCHES_REGEX
    - macro: '{#IFTYPE}'
      value: '24'                 # 排除环回接口类型
      operator: NOT_EQUALS
```
只监控特定 VLAN 接口：
```
filter:
  conditions:
    - macro: '{#IFNAME}'
      value: '\.(100|200|300)$'  # VLAN 100, 200, 300
      operator: MATCHES_REGEX

```
使用复杂表达式：
```
filter:
  evaltype: FORMULA
  formula: (A and B) or C
  conditions:
    - macro: '{#IFNAME}'
      value: '^eth'
      operator: MATCHES_REGEX
      formulaid: A
    - macro: '{#IFSPEED}'
      value: '1000000000'         # 1Gbps
      operator: EQUALS
      formulaid: B
    - macro: '{#IFALIAS}'
      value: 'IMPORTANT'
      operator: LIKE
      formulaid: C
```

#### 3.2.3 Preprocessing（预处理）
预处理步骤在数据存储前对其进行转换：
```
preprocessing:
  - type: CHANGE_PER_SECOND      # 计算每秒变化率
  - type: MULTIPLIER              # 乘法运算
    parameters: ['8']             # 字节转比特
  - type: REGEX                  # 正则提取
    parameters:
      - 'Temperature: ([\d.]+)'
      - '\1'
```

支持的预处理类型：
|类型|说明|参数示例|
|--------|------|-------|
|MULTIPLIER|乘数|`['0.001']`|
|SIMPLE_CHANGE|简单变化|无参数|
|CHANGE_PER_SECOND|每秒变化率|无参数|
|REGEX|正则表达式|`['pattern', 'output']`|
|JSONPATH|JSON路径|`['$.value']`|
|SNMP_WALK_TO_JSON|SNMP Walk转JSON|`['{#MACRO}', 'oid', '0']`|
|HEX_TO_DECIMAL|十六进制转十进制|无参数|
|JAVASCRIPT|JavaScript脚本|`['return value * 100']`|

#### 3.2.4 Macros（宏）
宏用于动态替换配置中的值

用户宏 `{$MACRO}`：
```
macros:
  - macro: '{$SNMP.TIMEOUT}'
    value: '5'
  - macro: '{$CPU.UTIL.CRIT}'
    value: '90'
```
LLD 宏 `{#MACRO}`：

在发现过程中自动填充：

- `{#SNMPINDEX}`：SNMP 索引
- `{#IFNAME}`：接口名称
- `{#IFTYPE}`：接口类型
- 自定义宏：通过 discovery 配置定义
宏替换机制：

- 发现阶段：提取 LLD 宏值
- 展开阶段：将宏替换为实际值
- 优先级：LLD 宏 > 用户宏 > 默认值
## 四、插件配置详解
### 4.1 基础配置
#### 4.1.1 SNMP 连接参数
```
[[instances]]
# 目标设备列表
agents = [
    "192.168.1.1",                    # 简单 IP
    "192.168.1.2:161",                # 指定端口
    "udp://192.168.1.3:161",          # 指定协议
    "tcp://switch.example.com:161",   # TCP 传输
    "192.168.1.0/24",                 # CIDR 网段（自动扫描）
]

# SNMP 版本（1, 2, 3）
version = 2

# SNMPv1/v2c 参数
community = "public"

# SNMPv3 参数
username = "snmpuser"
security_level = "authPriv"          # noAuthNoPriv, authNoPriv, authPriv
auth_protocol = "SHA"                 # MD5, SHA, SHA224, SHA256, SHA384, SHA512
auth_password = "auth_pass_123"
priv_protocol = "AES"                 # DES, AES, AES192, AES256
priv_password = "priv_pass_456"
context_name = ""

# 连接参数
port = 161                           # 默认 SNMP 端口
timeout = "5s"                        # 超时时间
retries = 3                          # 重试次数
max_repetitions = 10                 # BULK 请求最大重复数

# UDP 套接字模式
unconnected_udp_socket = false       # 使用非连接模式（处理大量设备时更高效）
```
Agents 配置格式说明：
- 支持多种格式混合使用
- CIDR 网段会自动展开为单个 IP(全量IP)
- 默认使用 UDP 协议和 161 端口
#### 4.1.2 模板加载
方式一：加载外部文件
```toml
template_files = [
    "/etc/categraf/templates/cisco_switch.yaml",
    "/etc/categraf/templates/interface_addon.yaml"
]
```
方式二：内嵌模板内容
```toml
[instances.template_file_contents]
# 直接在配置中嵌入模板内容
basic_template = '''
zabbix_export:
  version: '6.0'
  templates:
    - template: Embedded Template
      items:
        - name: System uptime
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.1.3.0
          key: system.uptime
'''
```
多模板合并规则：
- 后加载的模板覆盖前面的同名项
- 宏定义按名称合并
- 发现规则独立处理
### 4.2 高级配置
#### 4.2.1 发现功能配置

发现功能会自动从模板中读取 discovery_rules，发现调度机制：
- 每个发现规则按其 delay 独立调度
- 首次启动时立即执行一次发现
- 发现结果会缓存，避免重复执行

4.2.2 健康检查

健康检查自动进行，默认参数：
- 检查间隔：30秒
- 检查超时：5秒
- 最大重试：3次
- 自动重连：启用

健康检查是通过计数标记方式进行的，默认参数下，设备标记位不健康会通过3次检查来完成. 如下：

第一次检查失败--30秒-->第二次检查失败--30秒-->第三次检查失败，达到最大重试次数，标记设备为不健康。

## 五、数据类型处理机制
### 5.1 SNMP 数据类型映射
|SNMP PDU 类型|	Go 类型|	处理方式|
|--------|------|-------|
|Integer|int|直接使用|
|Counter32|uint32|转为 uint64|
|Counter64|uint64|直接使用|
|Gauge32|uint32|直接使用|
|TimeTicks|uint32|转换为秒（÷100）|
|OctetString|[]byte|自动识别可打印字符串|
|ObjectIdentifier|string|字符串表示|
|IPAddress|string|点分十进制表示|
|Opaque|[]byte|十六进制字符串|

### 5.2 特殊类型处理
#### 5.2.1 CHAR/TEXT 类型作为标签
当监控项的 value_type 为 CHAR 或 TEXT 时，插件会将其作为标签而非指标值：
```yaml
item_prototypes:
  - name: Interface {#IFNAME} alias
    type: SNMP_AGENT
    snmp_oid: .1.3.6.1.2.1.31.1.1.1.18.{#SNMPINDEX}
    key: net.if.alias[{#IFNAME}]
    value_type: TEXT              # 文本类型
```
Label Provider 机制：
- 识别 CHAR/TEXT 类型的监控项
- 提取标签键（从 key 中解析，如 net.if.alias -> net_if_alias）
- 缓存标签值并关联到相同索引的其他监控项
- 在输出指标时自动添加这些标签

对于一些枚举的CHAR或者TEXT类型的item, 最佳实践是通过预处理将其转换为数值。比如
```yaml
items:
  - name: "Interface {#IFNAME}: Operational status"
    key: "net.if.status[{#IFNAME}]"
    type: SNMP_AGENT
    snmp_oid: ".1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}"
    value_type: UNSIGNED  # 改为数值类型
    preprocessing:
      - type: JAVASCRIPT
        parameters:
          - |
            // 将 SNMP 返回的状态值转换为标准数值
            // IF-MIB::ifOperStatus 值:
            // 1 = up, 2 = down, 3 = testing, 4 = unknown,
            // 5 = dormant, 6 = notPresent, 7 = lowerLayerDown
            
            var statusMap = {
              '1': 1,  // up -> 1
              '2': 0,  // down -> 0
              '3': 0,  // testing -> 0
              '4': 0,  // unknown -> 0
              '5': 0,  // dormant -> 0
              '6': 0,  // notPresent -> 0
              '7': 0   // lowerLayerDown -> 0
            };
            
            return statusMap[value] !== undefined ? statusMap[value] : 0;
```

#### 5.2.2 Counter 类型处理
计数器类型会自动处理溢出：
```yaml
preprocessing:
  - type: CHANGE_PER_SECOND       # 自动处理计数器溢出
    # 32位计数器最大值：4294967295
    # 64位计数器最大值：18446744073709551615
```
速率计算公式：
- 正常情况：(新值 - 旧值) / 时间差
- 溢出情况：(最大值 - 旧值 + 新值) / 时间差
#### 5.2.3 OctetString 处理
OctetString 的处理取决于内容：
- 可打印字符串：直接转换为 string
- 二进制数据：转换为十六进制
- 特殊处理（通过预处理）：
    - MAC 地址：MAC_FORMAT
    - IP 地址：IP_FORMAT
    - 十六进制数值：HEX_TO_DECIMAL
## 六、自动发现功能
### 6.1 自动发现流程
```
1. 执行 SNMP Walk
   ↓
2. 提取索引和宏值
   ↓
3. 应用过滤器
   ↓
4. 生成监控项
   ↓
5. 动态调度采集
```
### 6.2 支持的发现类型
#### 6.2.1 网络接口发现
```
discovery_rules:
  - name: Network interfaces discovery
    snmp_oid: .1.3.6.1.2.1.2.2.1.2     # ifDescr
    # 或使用多 OID 发现
    snmp_oid: discovery[{#IFNAME},.1.3.6.1.2.1.2.2.1.2,{#IFTYPE},.1.3.6.1.2.1.2.2.1.3]
```
自动生成的宏：
- {#SNMPINDEX}：接口索引
- {#IFNAME}：接口名称
- {#IFTYPE}：接口类型
#### 6.2.2 文件系统发现
```yaml
discovery_rules:
  - name: Mounted filesystem discovery
    snmp_oid: .1.3.6.1.2.1.25.2.3.1.3  # hrStorageDescr
```
#### 6.2.3 自定义发现
使用 walk[] 语法执行多个 OID walk：
```yaml
discovery_rules:
  - name: Custom discovery
    snmp_oid: walk[.1.3.6.1.4.1.9.9.48.1.1.1.2,.1.3.6.1.4.1.9.9.48.1.1.1.5]
    preprocessing:
      - type: SNMP_WALK_TO_JSON
        parameters:
          - '{#VLANID}'
          - '.1.3.6.1.4.1.9.9.48.1.1.1.2'
          - '0'
          - '{#VLANNAME}'
          - '.1.3.6.1.4.1.9.9.48.1.1.1.5'
          - '0'
```
举个例子，原始 SNMP Walk 数据：
```
.1.3.6.1.2.1.2.2.1.2.1 = "lo"
.1.3.6.1.2.1.2.2.1.2.2 = "eth0"
.1.3.6.1.2.1.2.2.1.2.3 = "eth1"
.1.3.6.1.2.1.2.2.1.3.1 = 24        # loopback type
.1.3.6.1.2.1.2.2.1.3.2 = 6         # ethernet type
.1.3.6.1.2.1.2.2.1.3.3 = 6         # ethernet type
.1.3.6.1.2.1.2.2.1.5.1 = 10000000  # 10 Mbps
.1.3.6.1.2.1.2.2.1.5.2 = 1000000000 # 1 Gbps
.1.3.6.1.2.1.2.2.1.5.3 = 10000000000 # 10 Gbps
```
配置：
```
discovery_rules:
  - name: Network interface discovery
    type: SNMP_AGENT
    key: net.if.discovery
    # 使用 walk[] 语法执行多个 OID walk
    snmp_oid: walk[.1.3.6.1.2.1.2.2.1.2,.1.3.6.1.2.1.2.2.1.3,.1.3.6.1.2.1.2.2.1.5]
    preprocessing:
      - type: SNMP_WALK_TO_JSON
        parameters:
          - '{#IFNAME}'                # 宏名称
          - '.1.3.6.1.2.1.2.2.1.2'     # OID 基础
          - '0'                        # 批量提取标志 (0=单个值)
          - '{#IFTYPE}'                # 第二个宏
          - '.1.3.6.1.2.1.2.2.1.3'     # 第二个 OID
          - '0'
          - '{#IFSPEED}'               # 第三个宏
          - '.1.3.6.1.2.1.2.2.1.5'     # 第三个 OID
          - '0'
```
生成的 JSON：
```
[
  {
    "{#SNMPINDEX}": "1",
    "{#IFNAME}": "lo",
    "{#IFTYPE}": "24",
    "{#IFSPEED}": "10000000"
  },
  {
    "{#SNMPINDEX}": "2",
    "{#IFNAME}": "eth0",
    "{#IFTYPE}": "6",
    "{#IFSPEED}": "1000000000"
  },
  {
    "{#SNMPINDEX}": "3",
    "{#IFNAME}": "eth1",
    "{#IFTYPE}": "6",
    "{#IFSPEED}": "10000000000"
  }
]
```

### 6.3 发现数据处理流程
#### 6.3.1 OID Walk 执行
插件使用 BulkWalk 提高效率：
- 自动处理 SNMP v1 的 Walk
- v2c/v3 使用 BulkWalk
- 支持并发多个 OID walk
#### 6.3.2 宏值提取
从 OID 结果中提取：
```
OID: .1.3.6.1.2.1.2.2.1.2.1 = "eth0"
     └─ 基础 OID ─┘└─索引─┘   └值┘

提取结果：
{#SNMPINDEX} = "1"
{#IFNAME} = "eth0"
```
#### 6.3.3 过滤器应用
按照 filter 配置筛选发现的项目（见 3.2.2 Filter 部分）

#### 6.3.4 监控项生成
基于 item_prototypes 和宏值生成实际监控项：
```
模板：net.if.in[{#IFNAME}]
宏值：{#IFNAME} = "eth0"
结果：net.if.in[eth0]
```
## 七、预处理功能详解
### 7.1 支持的预处理类型列表
|类型|	用途|	示例|
|--------|------|-------|
|MULTIPLIER|数值乘法	|字节转比特（×8）|
|SIMPLE_CHANGE	|简单变化	|当前值-上次值|
|CHANGE_PER_SECOND	|速率计算	|流量速率|
|REGEX	|正则提取	|从字符串提取数值|
|JSONPATH	|JSON解析	|提取JSON字段|
|TRIM/LTRIM/RTRIM|字符串修剪|去除空白|
|JAVASCRIPT	|JS脚本|复杂逻辑处理|
|HEX_TO_DECIMAL	|进制转换	|0xFF -> 255|
|MAC_FORMAT	|MAC格式化|	标准化MAC地址|
|IP_FORMAT	|IP格式化|	提取IP地址|

### 7.2 常用预处理案例
#### 7.2.1 数值计算
字节转比特：
```yaml
preprocessing:
  - type: MULTIPLIER
    parameters: ['8']
```
百分比转换：
```yaml
preprocessing:
  - type: MULTIPLIER
    parameters: ['0.01']     # 如果原值是 0-10000，转为 0-100
```
计算速率：
```yaml
preprocessing:
  - type: CHANGE_PER_SECOND  # 自动计算每秒变化
```
#### 7.2.2 字符串处理
提取温度数值：
```yaml
preprocessing:
  - type: REGEX
    parameters:
      - 'Temperature: ([\d.]+)°C'
      - '\1'
```
去除空白：
```yaml
preprocessing:
  - type: TRIM               # 去除前后空白
```
#### 7.2.3 格式转换
MAC 地址格式化：
```yaml
preprocessing:
  - type: MAC_FORMAT
    parameters: [':']         # 分隔符（默认冒号）
# 输入：001122334455 或 00-11-22-33-44-55
# 输出：00:11:22:33:44:55
```
十六进制转十进制：
```yaml
preprocessing:
  - type: HEX_TO_DECIMAL
# 输入：FF 或 0xFF
# 输出：255
```
7.2.4 JavaScript 脚本

简单计算：
```yaml
preprocessing:
  - type: JAVASCRIPT
    parameters: ['return value * 100 / 1024']
```
条件处理：
```yaml
preprocessing:
  - type: JAVASCRIPT
    parameters:
      - |
        if (value > 1000000) {
            return value / 1000000;  // 转为 MB
        }
        return value / 1000;         // 转为 KB
```
字符串处理：
```yaml
preprocessing:
  - type: JAVASCRIPT
    parameters:
      - 'return value.toLowerCase().replace(/\s+/g, "_")'
```
#### 7.2.5 JSONPath 提取
提取 JSON 字段：
```yaml
preprocessing:
  - type: JSONPATH
    parameters: ['$.temperature.value']
```
提取数组元素：
```yaml
preprocessing:
  - type: JSONPATH
    parameters: ['$.interfaces[0].name']
```
## 八、实际配置示例
### 8.1 最小化配置示例
```toml
# /etc/categraf/conf/inputs.snmp_zabbix/snmp_zabbix.toml

[[instances]]
# 最简配置：监控单个设备的系统信息
agents = ["192.168.1.1"]
version = 2
community = "public"
template_files = ["/etc/categraf/templates/basic_system.yaml"]
```
对应的最简模板：
```yaml
# /etc/categraf/templates/basic_system.yaml
zabbix_export:
  version: '6.0'
  templates:
    - template: Basic System
      items:
        - name: System uptime
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.1.3.0
          key: system.uptime
          value_type: UNSIGNED
          preprocessing:
            - type: MULTIPLIER
              parameters: ['0.01']  # TimeTicks to seconds
```
### 8.2 完整配置示例
```toml
# /etc/categraf/conf/inputs.snmp_zabbix/snmp_zabbix.toml

[[instances]]
# 基础标签
labels = { region = "beijing", env = "production" }

# SNMP 连接配置
agents = [
    "192.168.1.0/24",           # 扫描整个网段
    "core-switch.example.com"   # 域名
]
version = 2
community = "public"
port = 161
timeout = "5s"
retries = 3
max_repetitions = 25

# 加载多个模板（会自动合并）
template_files = [
    "/etc/categraf/templates/cisco_catalyst.yaml",
    "/etc/categraf/templates/custom_oids.yaml"
]

# 设备映射标签
[instances.mappings]
"192.168.1.1" = { device_name = "core-sw-01", location = "DC1" }
"192.168.1.2" = { device_name = "core-sw-02", location = "DC2" }
```

## 8.3 常见场景配置
### 8.3.1 交换机端口监控
```toml
[[instances]]
agents = ["192.168.1.1"]
version = 2
community = "public"

# 使用 Cisco 官方模板
template_files = ["/etc/categraf/templates/net/cisco/cisco_snmp/template_net_cisco_snmp.yaml"]

# 或内嵌简化模板
[instances.template_file_contents]
switch_interfaces = '''
zabbix_export:
  version: '6.0'
  templates:
    - template: Switch Interfaces
      discovery_rules:
        - name: Interface discovery
          type: SNMP_AGENT
          key: net.if.discovery
          delay: 1h
          snmp_oid: discovery[{#IFNAME},.1.3.6.1.2.1.2.2.1.2,{#IFTYPE},.1.3.6.1.2.1.2.2.1.3,{#IFADMINSTATUS},.1.3.6.1.2.1.2.2.1.7]
          filter:
            evaltype: AND
            conditions:
              - macro: '{#IFTYPE}'
                value: '6'              # Ethernet
                operator: EQUALS
              - macro: '{#IFADMINSTATUS}'
                value: '1'              # UP
                operator: EQUALS
          item_prototypes:
            - name: 'Interface {#IFNAME}: Incoming traffic'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.2.2.1.10.{#SNMPINDEX}
              key: net.if.in[{#IFNAME}]
              value_type: UNSIGNED
              units: bps
              preprocessing:
                - type: CHANGE_PER_SECOND
                - type: MULTIPLIER
                  parameters: ['8']
            - name: 'Interface {#IFNAME}: Outgoing traffic'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.2.2.1.16.{#SNMPINDEX}
              key: net.if.out[{#IFNAME}]
              value_type: UNSIGNED
              units: bps
              preprocessing:
                - type: CHANGE_PER_SECOND
                - type: MULTIPLIER
                  parameters: ['8']
            - name: 'Interface {#IFNAME}: Description'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.31.1.1.1.18.{#SNMPINDEX}
              key: net.if.alias[{#IFNAME}]
              value_type: TEXT          # 作为标签
'''
```
#### 8.3.2 路由器监控
```toml
[[instances]]
agents = ["10.0.0.1"]
version = 3
username = "snmpv3user"
security_level = "authPriv"
auth_protocol = "SHA"
auth_password = "authpass123"
priv_protocol = "AES"
priv_password = "privpass456"

[instances.template_file_contents]
router_template = '''
zabbix_export:
  version: '6.0'
  templates:
    - template: Router Monitoring
      items:
        # CPU 使用率
        - name: CPU utilization
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.4.1.9.9.109.1.1.1.1.7.1
          key: system.cpu.util
          value_type: FLOAT
          units: '%'
        # 内存使用
        - name: Memory used
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.4.1.9.9.48.1.1.1.5.1
          key: vm.memory.used
          value_type: UNSIGNED
          units: B
        # 路由表大小
        - name: Routing table size
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.4.24.3.0
          key: net.routing.table.size
          value_type: UNSIGNED
      discovery_rules:
        # BGP 邻居发现
        - name: BGP peer discovery
          type: SNMP_AGENT
          key: bgp.peer.discovery
          snmp_oid: .1.3.6.1.2.1.15.3.1.2
          item_prototypes:
            - name: 'BGP peer {#PEER}: State'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.15.3.1.2.{#SNMPINDEX}
              key: bgp.peer.state[{#PEER}]
              value_type: UNSIGNED
'''
```
#### 8.3.3 存储设备监控
```
[[instances]]
agents = ["storage.example.com"]
version = 2
community = "public"

[instances.template_file_contents]
storage_template = '''
zabbix_export:
  version: '6.0'
  templates:
    - template: Storage Device
      discovery_rules:
        - name: Storage discovery
          type: SNMP_AGENT
          key: storage.discovery
          delay: 30m
          snmp_oid: discovery[{#STORAGEDESCR},.1.3.6.1.2.1.25.2.3.1.3,{#STORAGETYPE},.1.3.6.1.2.1.25.2.3.1.2]
          filter:
            conditions:
              - macro: '{#STORAGETYPE}'
                value: '.1.3.6.1.2.1.25.2.1.4'  # Fixed disk
                operator: EQUALS
          item_prototypes:
            - name: '{#STORAGEDESCR}: Total space'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.25.2.3.1.5.{#SNMPINDEX}
              key: vfs.fs.size[{#STORAGEDESCR},total]
              value_type: UNSIGNED
              units: B
              preprocessing:
                - type: MULTIPLIER
                  parameters: ['4096']  # 块大小
            - name: '{#STORAGEDESCR}: Used space'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.25.2.3.1.6.{#SNMPINDEX}
              key: vfs.fs.size[{#STORAGEDESCR},used]
              value_type: UNSIGNED
              units: B
              preprocessing:
                - type: MULTIPLIER
                  parameters: ['4096']
            - name: '{#STORAGEDESCR}: Usage in %'
              type: SNMP_AGENT
              snmp_oid: .1.3.6.1.2.1.25.2.3.1.6.{#SNMPINDEX}
              key: vfs.fs.pused[{#STORAGEDESCR}]
              value_type: FLOAT
              units: '%'
              preprocessing:
                - type: JAVASCRIPT
                  parameters:
                    - |
                      var used = value;
                      var total = 1000000;  // 需要从其他地方获取
                      return (used / total) * 100;
'''
```
#### 8.3.4 打印机监控
```
[[instances]]
agents = ["printer.example.com"]
version = 1  # 很多打印机只支持 v1
community = "public"

[instances.template_file_contents]
printer_template = '''
zabbix_export:
  version: '6.0'
  templates:
    - template: Printer Monitoring
      items:
        - name: Printer status
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.25.3.5.1.1.1
          key: printer.status
          value_type: UNSIGNED
        - name: Printer error state
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.25.3.5.1.2.1
          key: printer.error
          value_type: TEXT
        - name: Toner level black
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.43.11.1.1.9.1.1
          key: printer.toner.black
          value_type: UNSIGNED
          units: '%'
        - name: Pages printed
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.43.10.2.1.4.1.1
          key: printer.pages.total
          value_type: UNSIGNED
'''
```
## 九、故障排查
### 9.1 常见问题及解决方案
#### 9.1.1 连接问题
问题：无法连接到 SNMP 设备

检查步骤：
```bash 
# 1. 测试网络连通性
ping 192.168.1.1

# 2. 测试 SNMP 端口
nc -zvu 192.168.1.1 161

# 3. 使用 snmpwalk 测试
snmpwalk -v2c -c public 192.168.1.1 system

# 4. 检查防火墙
sudo iptables -L -n | grep 161
```
常见原因：
- 防火墙阻止 UDP 161 端口
- SNMP 服务未启动
- Community 字符串错误
- ACL 限制访问
#### 9.1.2 认证问题
SNMPv3 认证失败

检查配置：
```toml
# 确保所有参数匹配
username = "snmpuser"
security_level = "authPriv"
auth_protocol = "SHA"      # 大小写敏感
auth_password = "password"  # 至少8个字符
priv_protocol = "AES"
priv_password = "password"  # 至少8个字符
```
测试命令：
```bash
snmpget -v3 -l authPriv -u snmpuser -a SHA -A authpass123 -x AES -X privpass456 192.168.1.1 sysDescr.0
```
#### 9.1.3 OID 不存在
错误：OID not found on device

排查方法：
```bash
# 1. 列出设备支持的所有 OID
snmpwalk -v2c -c public 192.168.1.1 .1

# 2. 检查特定 OID
snmpget -v2c -c public 192.168.1.1 .1.3.6.1.2.1.2.2.1.10.1

# 3. 查看 MIB 支持
snmpwalk -v2c -c public 192.168.1.1 sysORTable
```
解决方案：
- 确认设备支持该 MIB
- 使用正确的 OID
- 某些设备需要启用特定 MIB
#### 9.1.4 发现失败
发现规则未返回任何项目

调试步骤：

手动执行 walk：
```bash
snmpwalk -v2c -c public 192.168.1.1 .1.3.6.1.2.1.2.2.1.2
```
检查过滤器：
```yaml
filter:
  conditions:
    - macro: '{#IFNAME}'
      value: 'eth'    # 可能过滤太严格
      operator: LIKE
```
查看日志：
```bash
tail -f /var/log/categraf/categraf.log | grep -i discovery
```
#### 9.1.5 预处理错误
预处理失败的常见原因：

正则表达式错误：
```yaml
# 错误：未转义特殊字符
- type: REGEX
  parameters: ['Temp: (\d+).(\d+)', '\1.\2']

# 正确：
- type: REGEX
  parameters: ['Temp: (\d+)\.(\d+)', '\1.\2']
```
JavaScript 语法错误：
```yaml
# 错误：缺少 return
- type: JAVASCRIPT
  parameters: ['value * 100']

# 正确：
- type: JAVASCRIPT
  parameters: ['return value * 100']
```
类型不匹配：
```yaml
# 错误：对字符串使用数值运算
- type: MULTIPLIER
  parameters: ['8']

# 正确：先转换类型
- type: REGEX
  parameters: ['(\d+)', '\1']
- type: MULTIPLIER
  parameters: ['8']
```
### 9.2 调试模式使用
启用调试模式：
```
# 启动时添加 debug 参数
./categraf --debug --inputs snmp_zabbix

# 查看详细日志
tail -f /var/log/categraf/categraf.log
```
### 9.3 日志分析
关键日志标识：
- E! - 错误
- W! - 警告
- I! - 信息
- D! - 调试
常见日志分析：
```bash
# 查看错误
grep "E!" /var/log/categraf/categraf.log

# 查看发现相关
grep -i discovery /var/log/categraf/categraf.log

# 查看特定设备
grep "192.168.1.1" /var/log/categraf/categraf.log

# 查看预处理错误
grep -i preprocessing /var/log/categraf/categraf.log
```
### 9.4 性能问题排查
采集延迟或超时

优化建议：

调整超时和重试：
```toml
timeout = "10s"        # 增加超时
retries = 2           # 减少重试
max_repetitions = 10  # 减少批量大小
```
减少并发请求：
```toml
# 分散不同设备的采集时间
[[instances]]
agents = ["192.168.1.1"]

[[instances]]
agents = ["192.168.1.2"]
```
优化发现规则：
```yaml
discovery_rules:
  - delay: 6h          # 减少发现频率
    filter:
      conditions:      # 严格过滤，减少生成的监控项
        - macro: '{#IFTYPE}'
          value: '6'
          operator: EQUALS
```

### 9.5 标签relabel
跟snmp插件相比，默认的设备标签从agent_host变成了snmp_agent , 如果你想修改,假如你想把key从snmp_agent修改回agent_host, 可以添加如下配置
```
[[instances.relabel_configs]]
source_labels = ["snmp_agent"]
target_label = "agent_host"
replacement = '$1'
action = "replace"

[[instances.relabel_configs]]
regex = "snmp_agent"
action = "labeldrop"
```

## 十、限制和注意事项
### 10.1 功能限制
#### 10.1.1 只支持 SNMP_AGENT 类型
插件只处理以下类型的监控项：
 - SNMP_AGENT 
 - SNMPV1_AGENT
 - SNMPV3_AGENT)
不支持的类型（会被忽略）：
- ZABBIX_AGENT
- HTTP_AGENT
- CALCULATED
- DEPENDENT
- TRAP
#### 10.1.2 不支持的 Zabbix 功能
|功能|	支持情况|	说明|
|--------|------|-------|
|Items|	✅ 部分支持|	仅 SNMP 类型|
|Discovery|	✅ 支持	|完整支持|
|Triggers|	❌ 不支持|	插件不处理告警|
|Graphs|	❌ 不支持|	忽略图表定义|
|Dashboards|	❌ 不支持	|忽略仪表板|
|Actions|	❌ 不支持|	不执行动作|
|Trends	|❌ 不支持|	不存储趋势数据|
|Events	|❌ 不支持	|不生成事件|

#### 10.1.3 模板版本兼容性
- 完全支持：Zabbix 6.0+ YAML 格式
- 不支持：Zabbix 5.x 及以下的 XML 格式
- 部分支持：可能无法识别最新版本的新特性

### 10.2 性能考虑
建议限制：
- 单个实例最多监控 100 个设备
- 每个设备最多 1000 个监控项
- 发现规则生成的项目不超过 10000 个
- 批量请求大小固定为 60 个 OID
资源消耗：
- 每个设备一个 SNMP 连接
- 每个监控项占用约 1KB 内存
- CPU 使用主要在预处理阶段
### 10.3 安全建议
使用 SNMPv3：
```toml
version = 3
security_level = "authPriv"
```
限制 community 权限：
- 使用只读 community
- 配置设备 ACL

网络隔离：
- SNMP 流量不应跨越不信任网络
- 使用 VLAN 隔离管理网络

定期更新：
- 及时更新 Categraf
- 更新设备固件

## 十一、迁移指南
### 11.1 从原生 SNMP 插件迁移
步骤 1：导出现有配置
原生 snmp插件 配置示例：
```
[[instances]]
agents = ["192.168.1.1"]
version = 2
community = "public"

[[instances.field]]
oid = ".1.3.6.1.2.1.1.3.0"
name = "uptime"

[[instances.field]]
oid = ".1.3.6.1.2.1.2.2.1.10.1"
name = "interface.eth0.in"
```
步骤 2: 转换为模板格式
创建模板文件 migration_template.yaml：
```yaml
zabbix_export:
  version: '6.0'
  templates:
    - template: Migrated from SNMP
      items:
        - name: System Uptime
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.1.3.0
          key: uptime
          value_type: UNSIGNED
        - name: Interface eth0 In
          type: SNMP_AGENT
          snmp_oid: .1.3.6.1.2.1.2.2.1.10.1
          key: interface.eth0.in
          value_type: UNSIGNED
```
步骤 3: 添加snmp_zabbix插件配置文件
```toml
# 新的 snmp_zabbix 配置
[[instances]]
agents = ["192.168.1.1"]
version = 2
community = "public"
template_files = ["new_template.yaml"]
```

### 11.2 从 Zabbix 迁移
步骤 1：导出 Zabbix 配置
推荐使用 Web 界面（见 2.2.1）

步骤 2：分析和筛选模板
```bash
# 查找包含 SNMP 项的模板
grep -l "type: SNMP" templates/*.yaml

# 统计每个模板的 SNMP 项数量
for f in templates/*.yaml; do
    count=$(grep -c "type: SNMP" "$f" 2>/dev/null || echo 0)
    if [ $count -gt 0 ]; then
        echo "$f: $count SNMP items"
    fi
done
```
步骤 3：配置映射表
如果 Zabbix 中使用了主机变量，创建映射：
```toml
[instances.mappings]
"192.168.1.1" = {
    device_name = "core-sw-01",
    location = "DC1",
    contact = "admin@example.com"
}

```
步骤 4：验证迁移
```bash
# 测试配置
categraf --test --inputs snmp_zabbix

# 与zabbix对比指标
```

## 十二、附录
A. 配置参数速查表

|参数	|类型	|默认值	|说明|
|--------|------|-------|-------|
|agents	|[]string	|必填	|目标设备列表|
|version	|int	|2	|SNMP 版本(1,2,3)|
|community	|string	|public	|团体字符串|
|username	|string	|-	|SNMPv3 用户名|
|security_level|	string	|noAuthNoPriv	|安全级别|
|auth_protocol|	string	|MD5	|认证协议|
|auth_password|	string	|-	|认证密码|
|priv_protocol|	string	|DES	|加密协议|
|priv_password|	string	|-	|加密密码|
|port|	int	|161	|SNMP 端口|
|timeout	|duration	|5s	|超时时间|
|retries	|int	|3	|重试次数|
|max_repetitions	|int	|10	|BULK单次请求返回的数据|
|template_files|	[]string	|-	|模板文件路径|

B. 预处理类型对照表

|Zabbix 类型|插件支持|	说明|
|--------|------|-------|
|MULTIPLIER|✅|	乘数|
|SIMPLE_CHANGE|✅|	简单变化|
|CHANGE_PER_SECOND|✅|	每秒变化率|
|REGEX|✅|	正则表达式|
|JSONPATH|✅|	JSON 路径|
|SNMP_WALK_TO_JSON|✅|	Walk 转 JSON|
|HEX_TO_DECIMAL|✅|	十六进制转十进制|
|JAVASCRIPT|✅|	JavaScript|
|TRIM	|✅|	去除空白|
|MAC_FORMAT|✅	|MAC 格式化|
|IP_FORMAT|	-	✅	|IP 格式化|

C. 常用 OID 列表
系统信息：
```
.1.3.6.1.2.1.1.1.0 - sysDescr
.1.3.6.1.2.1.1.3.0 - sysUpTime
.1.3.6.1.2.1.1.5.0 - sysName
.1.3.6.1.2.1.1.6.0 - sysLocation
.1.3.6.1.2.1.1.7.0 - sysServices
```
网络接口：
```
.1.3.6.1.2.1.2.2.1.2 - ifDescr
.1.3.6.1.2.1.2.2.1.3 - ifType
.1.3.6.1.2.1.2.2.1.5 - ifSpeed
.1.3.6.1.2.1.2.2.1.7 - ifAdminStatus
.1.3.6.1.2.1.2.2.1.8 - ifOperStatus
.1.3.6.1.2.1.2.2.1.10 - ifInOctets
.1.3.6.1.2.1.2.2.1.16 - ifOutOctets
```
CPU/内存（企业 MIB）：

```
# Cisco
.1.3.6.1.4.1.9.9.109.1.1.1.1.7 - CPU 使用率
.1.3.6.1.4.1.9.9.48.1.1.1.5 - 内存已用
.1.3.6.1.4.1.9.9.48.1.1.1.6 - 内存空闲

# HP
.1.3.6.1.4.1.11.2.14.11.5.1.9.6.1 - CPU 使用率
```
D. 正则表达式示例
```
# 提取数字
- type: REGEX
  parameters: ['(\d+)', '\1']

# 提取温度值
- type: REGEX
  parameters: ['Temperature:\s*(\d+\.?\d*)', '\1']

# 提取接口名称
- type: REGEX
  parameters: ['([\w-]+)\s*:\s*(.+)', '\1']

# 提取 IP 地址
- type: REGEX
  parameters: ['(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})', '\1']

# 提取 MAC 地址
- type: REGEX
  parameters: ['([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})', '\0']
```

E. JavaScript 脚本模板
```
// 基本计算
return value * 100;

// 条件判断
if (value > 1000) {
    return value / 1000;
} else {
    return value;
}

// 字符串处理
return value.toUpperCase();
return value.replace(/\s+/g, '_');

// JSON 处理
var obj = JSON.parse(value);
return obj.temperature;

// 数组处理
var parts = value.split(',');
return parts[0];

// 复杂逻辑
function convertBytes(bytes) {
    if (bytes >= 1099511627776) {
        return (bytes / 1099511627776).toFixed(2) + " TB";
    } else if (bytes >= 1073741824) {
        return (bytes / 1073741824).toFixed(2) + " GB";
    } else if (bytes >= 1048576) {
        return (bytes / 1048576).toFixed(2) + " MB";
    } else if (bytes >= 1024) {
        return (bytes / 1024).toFixed(2) + " KB";
    } else {
        return bytes + " B";
    }
}
return convertBytes(value);
```

F. 术语表

|术语|	说明|
|--|--|
|OID	|Object Identifier，对象标识符|
|MIB	|Management Information Base，管理信息库|
|PDU	|Protocol Data Unit，协议数据单元|
|LLD	|Low-Level Discovery，低级别发现|
|SNMP Walk	|遍历 SNMP 子树的操作|
|Community	|SNMPv1/v2c 的认证字符串|
|Bulk Request	|SNMPv2c/v3 的批量请求|
|Trap	|SNMP 主动推送的告警|
|Counter	|累加计数器，会溢出|
|Gauge|	测量值，可增可减|
|TimeTicks|	时间计数器，单位 1/100 秒|
|Item|	监控项，定义要采集的指标|
|Item Prototype|	项目原型，发现后生成监控项的模板|
|Macro	|宏，用于动态替换的变量|
|Preprocessing	|预处理，数据采集后的转换步骤|
