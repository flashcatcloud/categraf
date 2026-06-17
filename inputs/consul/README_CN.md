# Consul 采集插件

该插件用于收集 Consul 注册的所有健康检查的状态信息以及 Consul 自身的集群状态。插件通过 [Consul API](https://www.consul.io/docs/agent/http/health.html#health_state) 获取数据。

## 配置说明

```toml
# 采集 Consul 中注册的服务和健康检查状态
[[instances]]
  ## Consul Server 地址
  # address = "localhost:8500"

  ## Consul Server 的 URI 协议，支持 "http" 或 "https"
  # scheme = "http"

  ## 发送请求时使用的 ACL token
  # token = ""

  ## HTTP 基础认证 (Basic Authentication) 的用户名和密码
  # username = ""
  # password = ""

  ## 指定查询的数据中心 (Datacenter)
  # datacenter = ""

  ## 可选的 TLS 配置
  # tls_ca = "/etc/categraf/ca.pem"
  # tls_cert = "/etc/categraf/cert.pem"
  # tls_key = "/etc/categraf/key.pem"
  ## 忽略自签证书的安全校验
  # insecure_skip_verify = true
```

## 采集指标

| 指标名                          | 说明                                                                                                  |
| ----------------------------- | ----------------------------------------------------------------------------------------------------- |
| `consul_up`                     | 上一次对 Consul 的查询是否成功 (1 为成功，0 为失败)。                                                              |
| `consul_scrape_use_seconds`     | 抓取耗时 (秒)。                                                                                   |
| `consul_raft_peers`             | Raft 集群中 Peer (Server) 的数量。                                                     |
| `consul_raft_leader`            | 根据当前节点的状态，Raft 集群是否有 Leader。                                             |
| `consul_serf_lan_members`       | LAN 集群中的成员数量。                                                                  |
| `consul_serf_lan_member_status` | 集群成员状态。1=Alive, 2=Leaving, 3=Left, 4=Failed。                                |
| `consul_serf_wan_member_status` | WAN 集群中的成员状态。1=Alive, 2=Leaving, 3=Left, 4=Failed。                            |
| `consul_catalog_services`       | 集群中的服务数量。                                                                 |
| `consul_service_tag`            | 服务的标签 (Tags)。                                                                                    |
| `consul_health_node_status`     | 节点相关的健康检查状态。                                                       |
| `consul_health_service_status`  | 服务相关的健康检查状态。                                                    |
| `consul_service_checks`         | 链接 Service ID 和 Check Name (如果可用)。                                                      |
| `consul_catalog_kv`             | Consul KV (Key/Value) 存储中的数据 (只收集值为数值类型的 Key)。 |

同时，还会暴露部分 Consul Agent 原生指标，具体详见 [Agent Metrics](https://developer.hashicorp.com/consul/api-docs/agent#view-metrics)。
