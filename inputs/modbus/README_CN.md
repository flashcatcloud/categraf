# Modbus 插件

此插件通过 TCP 或 RTU 协议采集 Modbus 设备数据。

> **Fork 声明**: 本插件从 [InfluxData Telegraf's Modbus plugin](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/modbus) 移植并修改（上游 Commit: `65e95e6fd4e7e9ca1636102d8aebe7870694ebad`）。

## 配置

请参考配置文件样例 `conf/input.modbus/modbus.toml` 以了解所有可用配置项。

### 与 Telegraf 的兼容性

本插件高度兼容 Telegraf 的 `[[inputs.modbus]]` 配置：
- 完全支持三种配置风格（`register`, `request`, `metric`）。
- 保留所有 Telegraf 兼容选项，例如 `close_connection_after_gather` 和 `pause_between_requests`。

### Categraf 标签覆盖逻辑

标签优先级严格如下：
`内置标签 (name/type/slave_id)` < `Modbus 自定义标签` < `Categraf 实例标签`

注意：Categraf 的实例标签具有最高优先级（支持将标签值配置为 `"-"` 以删除已有标签）。全局标签 (`[global.labels]`) 和 `agent_hostname` 仅在缺失时进行补充，不会覆盖任何已有标签。

## 指标

默认的测量集 (measurement) 名称为 `modbus`。在 `metric` 配置风格下，如果提供了自定义的 `measurement` 名称，那么在 Categraf 中最终输出的指标名将会是 `{measurement}_{field_name}`。

### 指标样例

```
hvac_temperature 25.5
hvac_compressor_status 1
```
