# Dmesg 采集插件

该插件通过 `/dev/kmsg` 读取 Linux 内核消息，并统计包含内置错误关键字或用户自定义关键字的消息数量。关键字采用**区分大小写的子串匹配**。

**支持平台：** 仅支持 Linux。

## 运行条件

Categraf 进程必须能够打开并读取 `/dev/kmsg`，使用 root 用户运行通常可以获得所需权限。如果 Categraf 使用普通用户运行或部署在容器中，需要为服务或容器配置可用且可读的 `/dev/kmsg`。否则插件初始化会失败，并在 Categraf 日志中输出 `Error opening /dev/kmsg`。

## 配置说明

默认配置文件为 `conf/input.dmesg/dmesg.toml`。该文件默认没有启用任何 `[[instances]]`，因此插件默认不启动。保持默认配置执行 `./categraf --debug --inputs dmesg` 时，日志中会出现：

```text
W! no instances for input:dmesg
```

如需启用插件，取消 `[[instances]]` 的注释并按需配置：

```toml
# 采集间隔，单位为秒；不配置时使用全局采集间隔。
# interval = 15

[[instances]]
  # 实例采集间隔 = 全局/插件采集间隔 * interval_times。
  # interval_times = 1

  # 需要额外统计的关键字，采用区分大小写的子串匹配。
  # 请勿配置空字符串，否则每条消息都会匹配。
  external_keywords = [
    "I/O error",
    "task blocked for more than",
  ]
```

插件始终统计以下内置关键字：

- `Out of memory`
- `nf_conntrack: table full`
- `dropping packet`
- `will reset adapter`
- `memory error`
- `Reset successful for scsi`
- `Call Trace`
- `segfault`
- `NIC Link is Down`
- `EXT4-fs error`
- `Medium Error`
- `Package temperature above threshold`

## 采集指标

所有指标均以 `dmesg_` 为前缀：

| 指标 | 标签 | 说明 |
| --- | --- | --- |
| `dmesg_up` | 无 | 本次读取成功时为 `1`，读取 `/dev/kmsg` 失败时为 `0`。打开 `/dev/kmsg` 失败发生在初始化阶段，错误会记录在 Categraf 日志中。 |
| `dmesg_hit_keyword` | `keyword` | 包含对应关键字的内核消息累计数量。每个内置和自定义关键字都会产生一条时间序列，计数为 `0` 时也会上报。 |

`dmesg_hit_keyword` 在内存中累计，Categraf 或插件实例重启后会重新计数。插件启动时会先消费新打开的 `/dev/kmsg` 读取器中可用的内核消息，之后继续统计新产生的消息。

## 测试

请确保执行 Categraf 的用户有权读取 `/dev/kmsg`，然后运行：

```sh
./categraf --test --inputs dmesg
```
