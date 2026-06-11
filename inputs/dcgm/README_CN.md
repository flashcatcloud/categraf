# DCGM 采集插件

该插件用于采集 NVIDIA GPU 的核心监控指标，其底层集成了 NVIDIA 官方的 `dcgm-exporter` 逻辑。利用 Data Center GPU Manager (DCGM)，插件能够收集包括 GPU 温度、功率、显存使用率、核心利用率以及 XID 错误等详细的硬件统计数据。

> 注意：此插件仅在编译时带上 `dcgm` build tag (例如: `go build -tags "dcgm"`) 时才会生效。

## 配置说明

```toml
[[instances]]
  # 定义要抓取的 DCGM collectors 配置文件路径（用于定义哪些 FieldID 会被抓取）
  # 例如："/etc/categraf/dcgm/default-counters.csv"
  collectors = "/etc/categraf/dcgm/default-counters.csv"

  # 是否在 Kubernetes 环境下运行
  kubernetes = false
  # k8s gpu id 解析模式 (例如 "uid" 等)
  kubernetes-gpu-id-type = "uid"

  # 设置要监控的 GPU 设备范围，例如 "f" (flex), "g" (所有 GPU), "i" (GPU 实例) 
  devices = "f"

  # 设置是否启用假数据 (常用于测试)
  fake-gpus = false

  # 可选：连接到远端的 hostengine
  # remote-hostengine-info = "localhost:5555"

  # 直接在配置文件中内联声明 collector 文件内容
  # [instances.collector_files]
  # "/etc/categraf/dcgm/default-counters.csv" = """
  # DCGM_FI_DEV_GPU_TEMP, gauge, GPU temperature (in C)
  # DCGM_FI_DEV_POWER_USAGE, gauge, Power draw (in W).
  # """
```

## 采集指标

所有指标将附带如 `gpu`, `UUID`, `device` 等标签，常见的核心指标包括：

- `DCGM_FI_DEV_GPU_TEMP`: GPU 当前温度 (摄氏度)
- `DCGM_FI_DEV_POWER_USAGE`: GPU 实时功耗 (瓦特)
- `DCGM_FI_DEV_GPU_UTIL`: GPU 核心计算利用率 (%)
- `DCGM_FI_DEV_MEM_COPY_UTIL`: 显存读写利用率 (%)
- `DCGM_FI_DEV_FB_USED`: 已使用的显存大小 (MB)
- `DCGM_FI_DEV_FB_FREE`: 剩余空闲的显存大小 (MB)
- `DCGM_FI_DEV_XID_ERRORS`: GPU 发生的 XID 硬件/驱动错误次数

## 监控大盘

本插件提供了一个标准的 DCGM Dashboard 参考，主要涵盖 GPU 利用率、功耗、显存使用和温度监控。
