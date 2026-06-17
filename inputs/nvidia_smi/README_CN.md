# NVIDIA SMI 采集插件

该采集插件的原理是读取 `nvidia-smi` 命令行工具的输出，并转换为监控指标进行上报。它集成了 [nvidia_gpu_exporter](https://github.com/utkuozdemir/nvidia_gpu_exporter) 的核心代码。

**支持平台:** Linux, Windows (需安装 NVIDIA 显卡驱动并具备 `nvidia-smi` 命令)

## 配置说明

配置文件位于 `conf/input.nvidia_smi/nvidia_smi.toml`

```toml
# 采集 NVIDIA GPU 状态
# interval = 15

# 下面的配置是最核心的配置。如果要采集 nvidia-smi 的信息，请取消注释并给出 nvidia-smi 命令的绝对路径。
# 相当于让 Categraf 执行本机的 nvidia-smi 命令，获取本机 GPU 的状态信息
# nvidia_smi_command = "/usr/bin/nvidia-smi"

# 如果想远程采集远端机器的 GPU 状态，可以使用 ssh 命令登录远端机器执行。
# （由于 Categraf 通常是部署在每台物理机上的，因此绝大多数情况下不需要 SSH 方式）
# nvidia_smi_command = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null SSH_USER@SSH_HOST nvidia-smi"

# 逗号分隔的查询字段列表。你可以运行 `nvidia-smi --help-query-gpus` 来查看所有支持的字段。
# 填写 `AUTO` 将自动检测并采集支持的全部字段。
query_field_names = "AUTO"
```

## 采集指标

该插件支持采集数百种 GPU 指标（具体取决于驱动版本和显卡型号），所有的指标均以 `nvidia_smi_` 作为前缀，并默认带有 `uuid`、`name` (如 Tesla T4) 等显卡标识的标签。

重点关注的指标有：
- `nvidia_smi_utilization_gpu_ratio`: GPU 算力利用率 (0~1)
- `nvidia_smi_utilization_memory_ratio`: 显存带宽利用率 (0~1)
- `nvidia_smi_memory_used_bytes` / `nvidia_smi_memory_total_bytes`: 显存使用量与总量
- `nvidia_smi_temperature_gpu`: GPU 核心温度 (摄氏度)
- `nvidia_smi_power_draw_watts`: GPU 当前功耗
- `nvidia_smi_fan_speed_ratio`: 风扇转速百分比

## 监控大盘

本目录下提供了一个配套的基础 Dashboard (`dashboard.json`)，帮助您快速建立 GPU 的利用率、显存使用情况、温度与功耗的监控可视化体系。
