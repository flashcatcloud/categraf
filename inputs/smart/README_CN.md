# S.M.A.R.T. 采集插件

该插件通过命令行工具 `smartctl` 来采集 S.M.A.R.T. (Self-Monitoring, Analysis and Reporting Technology) 存储设备的硬件状态和健康指标。S.M.A.R.T. 是集成在硬盘（HDD）和固态硬盘（SSD）中的监控系统，用于检测并报告各种驱动器可靠性指标，旨在预测硬件故障。

该插件从 `telegraf/smart` fork 而来，并针对 Categraf 进行了适配和优化。

## 前置要求

- 操作系统中必须安装有 `smartmontools` (包含 `smartctl` 命令行工具)。
  - Ubuntu/Debian: `sudo apt-get install smartmontools`
  - CentOS/RHEL: `sudo yum install smartmontools`
- 运行 Categraf 的用户通常需要 `root` 权限才能读取磁盘 S.M.A.R.T. 信息。如果你希望以非 root 用户运行，可以通过配置 `sudo` 免密执行 `smartctl`，并在配置中开启 `use_sudo = true`。

## 配置说明

```toml
# 采集 S.M.A.R.T. 硬件状态指标
# interval = 60

[[instances]]
# 是否使用 sudo 执行 smartctl
# use_sudo = false

# (可选) 可选地提供一个特定的 smartctl 路径
# path_smartctl = "/usr/sbin/smartctl"

# (可选) 要采集监控的特定设备列表。
# 如果不指定（留空），插件会默认执行 `smartctl --scan` 自动发现系统上的所有磁盘。
# devices = [ "/dev/sda", "/dev/nvme0n1" ]

# 采集超时时间
# timeout = "5s"

# 是否采集 SMART 的底层具体 Attribute 详情（会生成更多详细指标）
attributes = true
```

## 采集指标

SMART 采集的指标被拆分为两个主要前缀（这取决于你是否开启了 `attributes`）：

### smart_device (设备通用概览指标)
- `smart_device_health_ok`: 磁盘健康度，1 为健康 (PASSED)，0 为异常
- `smart_device_temp_c`: 磁盘当前温度（摄氏度）
- `smart_device_power_on_hours`: 通电小时数
- `smart_device_power_cycle_count`: 电源循环（开关机）次数
- ...

### smart_attribute (详细 Attribute 指标)
如果开启了 `attributes = true`，则会为每一项特定的 SMART Attribute（如 Raw_Read_Error_Rate, Reallocated_Sector_Ct 等）生成以下指标：
- `smart_attribute_value`: 当前标准化数值
- `smart_attribute_worst`: 历史最差数值
- `smart_attribute_threshold`: 报警阈值
- `smart_attribute_raw_value`: 传感器原始记录数值 (通常是最具诊断意义的)

所有指标都会打上 `device` (如 `/dev/sda`) 以及具体的 `serial_no` (硬盘序列号) 标签。

## 监控大盘

本目录下提供了一个配套的 Dashboard (`dashboard.json`)，用于监控服务器磁盘的整体健康度 (Health PASSED/FAILED)、温度分布以及累计通电时间。
