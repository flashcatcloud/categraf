# ARP Packet 采集插件

该插件通过监听指定的网卡，使用 BPF 过滤器捕获 ARP 请求和响应包，从而统计本地 IP 地址发出的 ARP 包数量。

> 注意：运行该插件需要 Categraf 拥有捕获网络数据包的权限（例如 root 权限或 CAP_NET_RAW 权限），且系统依赖 libpcap。

## 配置说明

```toml
# 采集间隔时间 (单位: 秒)
interval = 15

[[instances]]
# 被监控端设备的网卡名称
eth_device = "eth0"
```

### 获取网卡名称

您可以使用以下命令获取可用的网卡名称列表：

```sh
ip addr | grep '^[0-9]' | awk -F':' '{print $2}'
```
示例输出：
```text
 lo
 eth0
 docker0
```

根据您的实际情况，将目标网卡（如 `eth0`）填入 `eth_device` 参数中。

## 采集指标

- `arp_packet_request_num`: 监听网卡上累计发出的 ARP 请求数
- `arp_packet_response_num`: 监听网卡上累计收到的 ARP 响应数

所有指标会附带标签 `sourceAddr`，表示绑定的本地 IPv4 地址。

## 测试

您可以使用以下命令单独测试该插件能否正常获取到值：

```sh
./categraf --test --inputs arp_packet
```
