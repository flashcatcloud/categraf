# hy_smi

该采集插件用于采集海光（Hygon）GPU 的监控指标，原理是通过执行 `hy-smi` 命令并解析其 JSON 输出，转换为监控数据上报。

## Configuration

配置文件在 `conf/input.hy_smi/hy_smi.toml`

```toml
# collect interval
interval = 15

# exec local command
hy_smi_command = "/opt/hyhal/bin/hy-smi --showid --showvbios --showdriverversion --showuse --showmemuse --showvoltage --showtemp --json"

# 如果想远程方式采集远端机器的 GPU 状态信息，可以使用 ssh 命令登录远端机器执行
# exec remote command
# hy_smi_command = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null SSH_USER@SSH_HOST hy-smi --showid --showvbios --showdriverversion --showuse --showmemuse --showvoltage --showtemp --json"

# query_timeout is used to set the query timeout to avoid the delay of data collection.
query_timeout = "5s"
```

## Metrics

- measurement: `hy_smi`
    - tags
        - `card` (GPU 卡名称，如 card0、card1)

    - fields
        - `gpu_info` (integer, 值为 1，携带 `device_id` 和 `vbios_version` 标签)
        - `temperature_sensor_edge_celsius` (float, 边缘温度，单位：摄氏度)
        - `temperature_sensor_junction_celsius` (float, 结点温度，单位：摄氏度)
        - `temperature_sensor_mem_celsius` (float, 显存温度，单位：摄氏度)
        - `temperature_sensor_core_celsius` (float, 核心温度，单位：摄氏度)
        - `hcu_use_ratio` (float, HCU 使用率，0~1 之间)
        - `hcu_memory_use_ratio` (float, HCU 显存使用率，0~1 之间)
        - `voltage_millivolts` (float, 电压，单位：毫伏)
        - `scraper_up` (integer, 采集是否成功，1 表示成功，0 表示失败)
        - `scrape_use_seconds` (float, 采集耗时，单位：秒)

## 前置条件

- 需要安装海光 GPU 驱动及 `hy-smi` 工具，通常位于 `/opt/hyhal/bin/hy-smi`
- `hy-smi` 命令需要支持 `--json` 参数输出 JSON 格式数据

## 手动验证

可以通过以下命令手动验证 `hy-smi` 是否正常工作：

```sh
/opt/hyhal/bin/hy-smi --showid --showvbios --showdriverversion --showuse --showmemuse --showvoltage --showtemp --json
```

预期输出类似：

```json
{
  "card0": {
    "Device ID": "0x6320",
    "VBIOS version": "6.312.002400Q.984920",
    "Driver version": "6.1.0",
    "Temperature (Sensor edge) (C)": "35",
    "Temperature (Sensor junction) (C)": "40",
    "Temperature (Sensor mem) (C)": "30",
    "Temperature (Sensor core) (C)": "33",
    "HCU use (%)": "10",
    "HCU memory use (%)": "5",
    "Voltage (mV)": "800"
  }
}
```

## Example Output

```text
hy_smi,card=card0 temperature_sensor_edge_celsius=35,temperature_sensor_junction_celsius=40,temperature_sensor_mem_celsius=30,temperature_sensor_core_celsius=33,hcu_use_ratio=0.1,hcu_memory_use_ratio=0.05,voltage_millivolts=800 1630572550000000000
hy_smi,card=card0,device_id=0x6320,vbios_version=6.312.002400Q.984920 gpu_info=1 1630572550000000000
hy_smi scraper_up=1,scrape_use_seconds=0.12 1630572550000000000
```

## TODO

GPU 卡已关注的监控指标，缺少监控大盘 JSON 和告警规则 JSON，欢迎大家 PR
