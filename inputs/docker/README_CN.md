# Docker 采集插件

Docker 采集插件用于收集本地运行的 Docker 容器的状态、CPU、内存、网络及块设备 I/O 等性能指标。该插件基于 `telegraf/inputs.docker` 进行改造 (fork)。

## 差异说明

与 Telegraf 官方插件的主要差异：
1. 使用了 `container_id` 作为指标的 Tag (Label)，而不是 Field，以方便更细粒度的聚合查询。
2. 精简了部分不常用的指标以降低时序数据库的存储压力。

## 配置说明

```toml
[[instances]]
  # Docker Daemon 的 API Endpoint
  # 支持 unix:// 或 tcp:// 协议
  endpoint = "unix:///var/run/docker.sock"

  # 采集超时时间
  timeout = "5s"

  # 控制是否启用 container_id 作为指标的标签
  container_id_label_enable = true

  # 是否截断 container_id (如果设为 true，则只取前 12 位)
  container_id_label_short_style = false
```

### 停用插件

如果你想停用该插件，有以下两种推荐方式：
- **方法一**：将 `conf/input.docker` 目录重命名（去掉 `input.` 前缀）。
- **方法二**：将配置中的 `endpoint` 字段留空。

## 常见问题解答 (FAQ)

### 1. 权限问题

Categraf 在尝试连接 `unix:///var/run/docker.sock` 时通常需要特权。建议使用 `root` 用户运行 Categraf。
如果您希望使用普通用户 (如 `categraf`) 运行，需要将该用户加入 `docker` 用户组：

```bash
sudo usermod -aG docker categraf
```

### 2. 在容器内部运行 Categraf

如果 Categraf 本身也是作为容器运行的，为了使其能够采集宿主机上的 Docker 信息，您必须将宿主机的 docker socket 挂载进容器：

**使用 Docker CLI:**
```bash
docker run -v /var/run/docker.sock:/var/run/docker.sock ...
```

**使用 Docker Compose:**
```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

## 采集指标

该插件支持对容器的资源使用情况进行全方位监控。部分核心指标如下：
- `docker_container_cpu_usage_percent`: 容器 CPU 使用率 (%)
- `docker_container_mem_usage_percent`: 容器内存使用率 (%)
- `docker_container_mem_limit`: 容器内存限制配额 (Bytes)
- `docker_container_net_rx_bytes`: 容器网络接收字节数 (Bytes)
- `docker_container_net_tx_bytes`: 容器网络发送字节数 (Bytes)
- `docker_container_status_*`: 容器状态相关字段，如 PID、退出码、重启次数和运行时长。当前状态会通过 `container_status` 标签体现。
