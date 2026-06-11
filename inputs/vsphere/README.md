# vSphere Input Plugin

This plugin uses the VMware vSphere API to collect performance metrics from ESXi hosts and virtual machines by connecting to your vCenter server.
It features automatic discovery of resources at various levels (Datacenters, Clusters, Hosts, VMs, and Datastores) and allows flexible filtering of resources and metrics using include/exclude rules.

## Configuration

```toml
# Collect VMware vSphere metrics
# interval = 60

[[instances]]
# vCenter connection URL (must include http/https)
vcenter = "https://vcenter.local/sdk"
username = "administrator@vsphere.local"
password = "yourpassword"

# TLS Configuration
# insecure_skip_verify = true

# ========================================================
# Resource Discovery & Filtering Controls
# You can enable/disable specific levels and configure filters
# ========================================================

# Datacenter level
datacenter_instances = true
# datacenter_include = ["/*"]
# datacenter_exclude = []
# datacenter_metric_include = ["/*"]

# Cluster level
cluster_instances = true
# cluster_include = ["/*/host/**"]

# Physical Host (ESXi) level
host_instances = true
# host_include = ["/*/host/**"]

# Virtual Machine (VM) level
vm_instances = true
# vm_include = ["/*/vm/**"]

# Datastore level
datastore_instances = true
# datastore_include = ["/*/datastore/**"]

# Use include and exclude rules to precisely control which resources are scraped, or to exclude unnecessary heavy metrics.
```

## Metrics

The vSphere performance metrics ecosystem is massive. This plugin generates metrics with different prefixes depending on the resource entity, such as:
- `vsphere_vm_*`: Virtual Machine metrics (e.g., `cpu_usage_average`, `mem_active_average`)
- `vsphere_host_*`: ESXi Host metrics
- `vsphere_datastore_*`: Datastore metrics
- `vsphere_cluster_*` / `vsphere_datacenter_*`: Macroscopic cluster and datacenter summary metrics

All metrics carry detailed topological context labels such as `datacentername`, `clustername`, `hostname`, `vmname`, etc.

## Dashboards

A comprehensive, pre-built `dashboard.json` is already provided in this directory. By importing this Dashboard, you gain immediate visibility into the overall capacity of your host clusters, resource utilization of individual ESXi hosts, and detailed CPU, Memory, and Disk I/O metrics for individual VMs.
