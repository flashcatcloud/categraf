# Redfish 采集插件

该插件用于采集支持 Redfish 协议的物理机带外管理（OOB）接口（如 Dell iDRAC、HPE iLO、Lenovo XClarity 等）的硬件传感器与状态指标。
相比于传统的 IPMI 协议，Redfish 基于现代的 HTTP/RESTful API，提供更丰富的 JSON 格式硬件指标。

## 配置说明

```toml
# 采集 Redfish 硬件状态指标
# interval = 60

[[instances]]
# 配置 Redfish 的连接地址、账户和密码
# [[instances.addresses]]
# url = "https://10.0.0.1"
# username = "admin"
# password = "password"
# (由于 Redfish 通常使用自签名证书，可以忽略 TLS 校验)
# insecure_skip_verify = true

# ================================
# 以下为指标采集路径 (Sets/Metrics) 的配置示例
# 插件会根据设定的 URN 和 JSON Path 解析出具体的数值型指标
# ================================

[[instances.sets]]
urn = "/redfish/v1/Chassis/System.Embedded.1/Thermal"
prefix = "thermal_"
[[instances.sets.metrics]]
name = "temperature"
path = "Temperatures.#.ReadingCelsius"
[[instances.sets.metrics.tags]]
name = "name"
path = "Temperatures.#.Name"

[[instances.sets]]
urn = "/redfish/v1/Chassis/System.Embedded.1/Power"
prefix = "power_"
[[instances.sets.metrics]]
name = "consumed_watts"
path = "PowerControl.#.PowerConsumedWatts"
```

## 采集指标

该插件的指标完全由配置文件中的 `sets` 和 `metrics` (基于 JSON Path 解析) 动态决定。通常我们会采集：

- **温度**: `redfish_thermal_temperature` (各传感器的摄氏度)
- **功耗**: `redfish_power_consumed_watts` (系统当前功耗)
- **风扇**: 风扇转速 (RPM 或百分比)
- **磁盘**: 物理磁盘与逻辑卷的健康状态
- **电源**: 冗余电源模块的运行状态

所有指标默认都会附带 Redfish 的请求地址等标签，也可以通过 `tags` 提取 JSON 中的名称字段作为 Label（例如提取 `Temperatures.#.Name` 作为传感器名字）。

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，可用于快速监控通过 Redfish 抓取到的服务器环境温度、整体功耗等关键硬件健康指标。
