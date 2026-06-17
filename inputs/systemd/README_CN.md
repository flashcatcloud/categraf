# systemd 插件

该插件用于采集 Linux 系统上 `systemd` 的运行状态、各个 unit (service, socket, timer 等) 的状态、重启次数、启动时间以及任务数等关键指标。

本插件的实现自 [node_exporter](https://github.com/prometheus/node_exporter/blob/master/collector/systemd_linux.go) fork 并经过修改适配。
**注意**：该插件通过纯 Go 语言实现与 D-Bus 的交互，不需要开启 CGO 即可在 Linux 环境下编译和运行。在非 Linux 系统下编译会退化为空实现。

## Configuration

在 Categraf 的配置文件中，可以通过以下选项来开启和配置 systemd 插件的采集（位于 `conf/input.systemd/systemd.toml`）：

```toml
# 是否启用该插件
enable = false 

# 正则表达式：用于匹配需要采集的 unit 名称，默认为匹配所有 (".+")
# unit_include = ".+"

# 正则表达式：用于匹配需要排除采集的 unit 名称。
# 如果一个 unit 同时符合 include 和 exclude 的正则，它将会被排除。
# 默认排除了 automount, device, mount, scope, slice 类型的 unit。
# unit_exclude = ".+\\.(automount|device|mount|scope|slice)"

# 是否建立一个私有的、不经过 dbus 的直连到 systemd (强烈不建议开启，需要 root 权限，主要用于测试)
# systemd_private = false

# 是否采集 service unit 的启动时间信息 (单位：秒)
enable_start_time_metrics = true 

# 是否采集 service unit task (任务数) 的指标
enable_task_metrics = true 

# 是否采集 service unit 重启的次数信息
enable_restarts_metrics = true 
```

## Metrics

插件成功采集后，会上报以下系统指标（所有的指标名称在系统中都会自动附带 `systemd_` 测量前缀）：

| 指标名称 | 类型 | 标签 (Tags) | 说明 |
| :--- | :--- | :--- | :--- |
| `systemd_version` | Gauge | `version` | 检测到的 systemd 版本。指标值为版本号浮点数，完整的版本字符串记录在 `version` 标签中。 |
| `systemd_system_running` | Gauge | 无 | 整个系统是否在正常运行 (类似命令 `systemctl is-system-running`)，值为 1.0 表示 running。 |
| `systemd_units` | Gauge | `state` | 处于不同系统状态 (`active`, `activating`, `deactivating`, `inactive`, `failed`) 的 unit 总计数量。 |
| `systemd_unit_state` | Gauge | `name`, `state`, `type` | 特定 unit 的状态指示器。如果该 unit 正处于对应的 `state` 则值为 1.0，否则为 0.0。 |
| `systemd_service_restart_total` | Counter | `name` | service 类型的 unit 所触发的重启总次数。 |
| `systemd_unit_start_time_seconds` | Gauge | `name` | unit 的启动时间点 (表示为自 Unix epoch 以来的秒数)。 |
| `systemd_unit_tasks_current` | Gauge | `name` | 当前 unit 内部正在运行的任务数量。 |
| `systemd_unit_tasks_max` | Gauge | `name` | 当前 unit 允许的最大任务数量。 |
| `systemd_socket_accepted_connections_total` | Counter | `name` | socket 类型的 unit 累计已接受的连接总数。 |
| `systemd_socket_current_connections` | Gauge | `name` | socket 类型的 unit 当前活动的连接数。 |
| `systemd_socket_refused_connections_total` | Gauge | `name` | socket 类型的 unit 累计被拒绝的连接总数 (需要 systemd >= 239)。 |
| `systemd_timer_last_trigger_seconds` | Gauge | `name` | timer 类型的 unit 上一次触发的时间点 (自 Unix epoch 以来的秒数)。 |