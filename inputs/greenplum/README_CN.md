# Greenplum 采集插件

Greenplum 采集插件用于监控 Greenplum 数据库集群的镜像节点 (Mirror) 状态。

> 注意：该插件依赖于运行环境中存在 `gpstate` 命令行工具。

## 采集原理

插件在后台会定期执行 `gpstate -m` 命令，并解析其输出中的 `Status` (运行状态) 和 `Data Status` (数据同步状态)。由于它直接调用 Greenplum 官方的管理工具，因此**必须使用有权限执行 `gpstate` 的用户（如 `gpadmin`）来运行 Categraf**，或者配置合适的免密环境。

## 配置说明

```toml
# # 采集周期
# interval = 15

[[instances]]
# 该插件没有实例级别的特殊配置。只需确保环境中有 gpstate 即可。
# 可以加一些标签来区分不同集群
# labels = { cluster="gp-cluster-1" }
```

## 采集指标

所有指标将附带 `Mirror` (镜像名称), `Datadir` (数据目录) 和 `Port` (端口) 作为标签。

- `greenplum_Status`: 节点状态。`1` 表示状态为 `Passive`，否则为 `0`。
- `greenplum_Data_Status`: 数据同步状态。`1` 表示状态为 `Synchronized` (已同步)，否则为 `0`。

## 监控大盘

建议将这些状态指标放入 Greenplum 的整体监控大盘中进行监控，当 `greenplum_Data_Status` 长时间为 `0` 时，说明主备数据同步可能存在异常，应触发告警。
