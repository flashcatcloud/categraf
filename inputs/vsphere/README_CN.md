# vSphere 采集插件

该插件使用 VMware vSphere API，通过连接到 vCenter 从 ESXi 主机和虚拟机集群中采集性能指标。
它支持自动发现 Datacenter、Cluster、Host、VM 和 Datastore 等各层级资源，并支持通过 include/exclude 规则对采集范围和指标进行灵活过滤。

## 配置说明

```toml
# 采集 VMware vSphere 指标
# interval = 60

[[instances]]
# vCenter 访问地址，需带协议 (http/https)
vcenter = "https://vcenter.local/sdk"
username = "administrator@vsphere.local"
password = "yourpassword"

# TLS 配置
# insecure_skip_verify = true

# ========================================================
# 资源层级发现与过滤控制
# 以下分为不同的资源层级，你可以开启/关闭该层级并配置过滤规则
# ========================================================

# 数据中心 (Datacenter)
datacenter_instances = true
# datacenter_include = ["/*"]
# datacenter_exclude = []
# datacenter_metric_include = ["/*"]

# 集群 (Cluster)
cluster_instances = true
# cluster_include = ["/*/host/**"]

# 物理主机 (Host)
host_instances = true
# host_include = ["/*/host/**"]

# 虚拟机 (Virtual Machine)
vm_instances = true
# vm_include = ["/*/vm/**"]

# 数据存储 (Datastore)
datastore_instances = true
# datastore_include = ["/*/datastore/**"]

# 你可以利用 include 和 exclude 来精细控制采集特定目录下的资源，或排除不需要的指标
```

## 采集指标

vSphere 的性能指标系统非常庞大。该插件会依据不同的资源实体，生成带不同前缀的指标，例如：
- `vsphere_vm_*`: 虚拟机指标 (例如 `cpu_usage_average`, `mem_active_average`)
- `vsphere_host_*`: ESXi 宿主机指标
- `vsphere_datastore_*`: 数据存储指标
- `vsphere_cluster_*` / `vsphere_datacenter_*`: 宏观集群与数据中心汇总指标

所有指标将附带详细的拓扑位置标签，例如 `datacentername`, `clustername`, `hostname`, `vmname` 等。

## 监控大盘

本目录下已经包含了一个非常丰富且完整的预置 `dashboard.json`。导入该 Dashboard，你可以直接查看宿主机集群的整体容量、各 ESXi 主机的水位，以及单个虚拟机的 CPU、内存和磁盘 I/O 监控。
