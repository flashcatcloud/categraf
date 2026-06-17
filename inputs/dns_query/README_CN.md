# DNS Query 采集插件

DNS Query 采集插件用于对 DNS 服务器的响应质量进行持续监测，帮助运维人员快速定位域名解析带来的网络延迟和解析错误问题。

**部署建议：**
不需要在所有机器上启用此插件，建议在核心网关节点、特定的网络探针虚拟机或复合监控节点上启用，定期拨测关键依赖的域名即可。

## 配置说明

```toml
[[instances]]
  # 当 servers 为空时，是否自动使用本机 /etc/resolv.conf 中的 DNS 服务器进行查询
  auto_detect_local_dns_server = true

  ## 手动指定要查询的 DNS 服务器
  servers = ["223.5.5.5", "114.114.114.114", "119.29.29.29"]

  ## 指定查询协议，如 "udp" 或 "tcp"
  # network = "udp"

  ## 需要重点监测的域名列表
  domains = ["www.huaweicloud.com", "www.baidu.com", "api.yourcompany.com"]

  ## 查询记录的类型 (A, AAAA, ANY, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV)
  record_type = "A"

  ## DNS 服务端口
  # port = 53

  ## DNS 查询的超时时间 (秒)
  timeout = 5
```

如果需要拨测不同类型的记录（如 `A` 记录和 `CNAME` 记录），可以配置多个 `[[instances]]` 块。

## 采集指标

- `dns_query_query_time_ms`: DNS 解析延迟时间 (毫秒)
- `dns_query_result_code`: 探测过程的结果码 (0 为成功，非 0 为异常，如超时、无法连接等)
- `dns_query_rcode_value`: DNS 协议标准返回的响应码 (如 NOERROR, NXDOMAIN, SERVFAIL 等)

所有指标都会带上 `server`, `domain`, `record_type` 等标签，方便按照特定 DNS 服务器或域名进行聚合分析。

## 告警建议

- 当 `dns_query_query_time_ms > 2000` 毫秒时，可以作为 P2 级别告警。
- 当 `dns_query_query_time_ms > 5000` 毫秒时，可以作为 P1 级别告警。
- 当 `dns_query_result_code != 0` 时，说明 DNS 解析失败，需立即介入。
