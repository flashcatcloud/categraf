# AMD ROCm System Management Interface (SMI) 采集插件

该插件 fork 自 [telegraf/amd_rocm_smi](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/amd_rocm_smi)

此插件通过执行 [`rocm-smi`][1] 命令来获取 AMD GPU 的状态指标，包括显存使用、GPU 使用率、温度等。

[1]: https://github.com/RadeonOpenCompute/rocm_smi_lib/tree/master/python_smi_tools

## 配置说明

```toml 
# 使用 rocm-smi 命令查询 AMD 显卡统计信息
# bin_path = "/opt/rocm/bin/rocm-smi"
# 如果不设置 bin_path，则不会进行采集

## 可选: GPU 轮询的超时时间
# timeout = "5s"
```

## 采集指标

- 测量名称: `amd_rocm_smi`
    - 标签 (Tags)
        - `name` (rocm-smi 可执行文件分配的显卡名称)
        - `gpu_id` (rocm-smi 识别的 GPU ID)
        - `gpu_unique_id` (GPU 的唯一 ID)

    - 字段 (Fields)
        - `driver_version` (整数)
        - `fan_speed` (整数，风扇转速百分比)
        - `memory_total` (整数 B，显存总量)
        - `memory_used` (整数 B，已用显存)
        - `memory_free` (整数 B，空闲显存)
        - `temperature_sensor_edge` (浮点数，摄氏度)
        - `temperature_sensor_junction` (浮点数，结温摄氏度)
        - `temperature_sensor_memory` (浮点数，显存温度摄氏度)
        - `utilization_gpu` (整数，GPU 使用率百分比)
        - `utilization_memory` (整数，显存使用率百分比)
        - `clocks_current_sm` (整数，Mhz)
        - `clocks_current_memory` (整数，Mhz)
        - `power_draw` (浮点数，瓦特)

## 故障排除

如果遇到问题，可以尝试手动运行完整的 `rocm-smi` 命令来检查输出结果。

Linux 环境下:

```sh
rocm-smi rocm-smi -o -l -m -M  -g -c -t -u -i -f -p -P -s -S -v --showreplaycount --showpids --showdriverversion --showmemvendor --showfwinfo --showproductname --showserial --showuniqueid --showbus --showpendingpages --showpagesinfo --showretiredpages --showunreservablepages --showmemuse --showvoltage --showtopo --showtopoweight --showtopohops --showtopotype --showtoponuma --showmeminfo all --json
```

如果在 GitHub 提交 issue，请附上此命令的输出结果以及您所使用的 ROCm 版本。
