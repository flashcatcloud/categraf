# IPVS 采集插件

该插件用于采集 Linux IPVS (IP Virtual Server) 的虚拟服务器 (Virtual Server) 和真实服务器 (Real Server) 的状态和网络流量指标。
它通过底层的 netlink socket 接口与 Linux 内核直接通信来获取数据。该插件 fork 自 telegraf。

**支持平台:** Linux

## 权限要求

为了通过 netlink socket 接口与内核通信，运行 Categraf 的进程需要 root 权限，或者至少具备 `CAP_NET_ADMIN` 和 `CAP_NET_RAW` 能力 (Capabilities)。在使用此插件前，请务必确保 Categraf 拥有足够的权限。

## 配置说明

```toml
# 采集 Linux IPVS 的虚拟和真实服务器指标
[[instances]]
# 无需任何特殊配置，只需启用即可
```

## 采集指标

采集的指标会自动打上标签，以标识虚拟服务器的配置方式（例如，使用 `address` + `port` + `protocol` 或者使用 `fwmark` 配置）。这与您平时使用 `ipvsadm` 配置虚拟服务器的方式一致。

### 1. ipvs_virtual_server
表示虚拟服务器 (负载均衡前端)。
- **Tags:**
  - `sched`: 使用的调度算法 (如 rr, wrr)
  - `netmask`: 掩码
  - `address_family`: inet 或 inet6
  - `address`: VIP 地址
  - `port`: 端口
  - `protocol`: 协议 (tcp/udp)
  - `fwmark`: 防火墙标记
- **Fields (指标):**
  - `connections`: 总连接数
  - `pkts_in` / `pkts_out`: 收发数据包总数
  - `bytes_in` / `bytes_out`: 收发字节总数
  - `pps_in` / `pps_out`: 每秒收发数据包速率
  - `cps`: 每秒新建连接数

### 2. ipvs_real_server
表示真实服务器 (后端的真实节点)。
- **Tags:**
  - `address`: Real Server IP
  - `port`: Real Server 端口
  - `address_family`: inet 或 inet6
  - `virtual_address` / `virtual_port` / `virtual_protocol` / `virtual_fwmark`: 其所属的虚拟服务器的信息
- **Fields (指标):**
  - `active_connections`: 活跃连接数
  - `inactive_connections`: 非活跃连接数
  - `connections`: 总连接数
  - `pkts_in` / `pkts_out`: 收发数据包总数
  - `bytes_in` / `bytes_out`: 收发字节总数
  - `pps_in` / `pps_out`: 每秒收发数据包速率
  - `cps`: 每秒新建连接数