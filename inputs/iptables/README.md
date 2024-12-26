# Iptables Plugin
forked from [telegraf/iptables](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/iptables)

iptables插件为一个集合中的规则收集数据包和字节计数器
来自Linux的iptables防火墙的表tables和链chains。

iptables的四种内建表：Filter, NAT, Mangle, Raw

表的处理优先级：raw>mangle>nat>filter。

filter：一般的过滤功能，是iptables的默认表，它有三种内建链(chains)：INPUT链 – 处理来自外部的数据；OUTPUT链 – 处理向外发送的数据；FORWARD链 – 将数据转发到本机的其他网卡设备上。

Nat：用于nat功能（端口映射，地址映射等）

Mangle：用于对特定数据包的修改

Raw：优先级最高，设置raw时一般是为了不再让iptables做数据包的链接跟踪处理，提高性能

默认表是filter（没有指定表的时候就是filter表）。


**规则通过相关注释进行标识。没有注释的规则将被忽略**


使用该插件前，必须确保要监控的规则名称带有唯一注释。注释可使用 `-m comment --comment "my comment"` iptables 选项添加。


iptables 命令需要 CAP_NET_ADMIN 和 CAP_NET_RAW 功能。您可以有几个选项可以授权 categraf 运行 iptables：

* 以root身份运行categraf。
* 将systemd配置为使用CAP_NET_ADMIN和CAP_NET_RAW运行categraf。这是最简单和推荐的选项。
* 将sudo配置为授予categraf以运行iptables。这是最多的限制性选项，但需要sudo设置。

## 使用 systemd 功能

您可以运行 `systemctl edit categraf.service`，并添加以下内容：

```shell
[Service]
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
```

由于 categraf 会用一个子进程来运行 iptables，因此需要 `AmbientCapabilities` 来向子进行赋予允许接口配置、管理IP防火墙和路由表、设置套接字的调试选项等能力。具体可以搜索Linux安全相关Capabilities机制

## 使用 sudo

您的 categraf 配置中需要以下内容：

```toml
[[instances]]
  use_sudo = true
```

您还需要更新 sudoers 文件：

```bash
$ visudo
# Add the following line:
Cmnd_Alias IPTABLESSHOW = /usr/bin/iptables -nvL *
categraf  ALL=(root) NOPASSWD: IPTABLESSHOW
Defaults!IPTABLESSHOW !logfile, !syslog, !pam_session
```

## 使用 IPtables 锁定功能

在 categraf.toml 中定义此插件的多个实例会导致同时访问 IPtables，导致 categraf.log 中出现 "ERROR in input [instances]: exit status 4 "信息并,丢失度量指标。在插件配置中设置 "use_lock = true “将使用”-w "开关运行 IPtables允许使用锁来防止出现此错误。

## 全局配置选项

除了插件配置特定项目外还支持与其它插件相同的通用功能，例如interval_times、labels、metrics_drop、metrics_pass、metrics_name_prefix等功能


## 配置

```toml @iptables.toml
# 从 iptables 收集数据包和字节吞吐量
# 该插件仅支持 Linux
[[instances]]
  ## 在大多数系统上，iptables 需要 root 访问权限。
  ## 将 "use_sudo "设为 true 将使用 sudo 运行 iptables。
  ## 用户必须配置 sudo，允许 categraf 的运行用户在运行 iptables 时不输入密码。
  ## 没有密码。
  ## 可以限制 iptables 只能使用列表命令 "iptables -nvL"。
  use_sudo = false
  ## 将 "use_lock "设为 true，使用 " -w "选项运行 iptables。
  ## 如果使用该选项，请适当调整你的 sudo 设置
  ## ("iptables -w 5 -nvl")
  use_lock = false
  ## 定义一个备用可执行文件，如 " ip6tables "。默认为 "iptables"。
  # binary = "ip6tables"
  ## 定义要监控的表:
  table = "filter"
  ## 定义要监控的链.
  ## 注意：不监控无注释的 iptables 规则。
  ## 请阅读插件文档了解更多信息。
  chains = [ "INPUT" ]
```

## Metrics指标

### Measurements测量值 & Fields数据类型

* iptables
  * pkts (integer, count)
  * bytes (integer, bytes)

### Tags

* 所有指标值都有以下tags:
  * table
  * chain
  * ruleid

`ruleid` 是与规则相关的注释。

## 输出示例

```shell
iptables -nvL INPUT
```

```text
Chain INPUT (policy DROP 0 packets, 0 bytes)
pkts bytes target     prot opt in     out     source               destination
100   1024   ACCEPT     tcp  --  *      *       192.168.0.0/24       0.0.0.0/0            tcp dpt:22 /* ssh */
 42   2048   ACCEPT     tcp  --  *      *       192.168.0.0/24       0.0.0.0/0            tcp dpt:80 /* httpd */
```

```text
iptables,table=filter,chain=INPUT,ruleid=ssh pkts=100i,bytes=1024i 1453831884664956455
iptables,table=filter,chain=INPUT,ruleid=httpd pkts=42i,bytes=2048i 1453831884664956455
```
