# Jenkins 采集插件

该插件用于采集 Jenkins 持续集成服务器的节点状态 (Node/Computer) 以及任务构建 (Job/Build) 状态。
它通过直接请求 Jenkins 的 JSON API 获取相关数据。

## 配置说明

```toml
# 采集周期
# interval = 60

[[instances]]
# Jenkins 服务的根 URL
jenkins_url = "http://localhost:8080"

# 认证凭据 (如果 Jenkins 未开启匿名访问，请务必提供有对应权限的账户密码/Token)
jenkins_username = "admin"
jenkins_password = "password_or_token"

# TCP 连接池的最大空闲连接数
# max_connections = 5
# HTTP 请求超时时间
# response_timeout = "5s"

# ===== Job/任务 过滤配置 =====
# 最大获取任务的历史层级 (控制扫描所有文件夹和子任务的深度)
# max_subjob_depth = 0
# 每层最多获取的任务数
# max_subjob_per_layer = 10
# 超过多长时间未构建的任务将被忽略
# max_build_age = "24h"

# 任务名称过滤，支持通配符
# job_include = []
# job_exclude = []

# ===== Node/节点 过滤配置 =====
# 节点名称过滤，支持通配符
# node_include = []
# node_exclude = []
```

## 采集指标

**全局与节点 (Node) 指标:**
- `jenkins_up`: 节点是否在线 (1:在线, 0:离线)
- `jenkins_busy_executors`: 整个 Jenkins 正在工作的执行器数量
- `jenkins_total_executors`: 整个 Jenkins 的总执行器数量
- `jenkins_node_num_executors`: 单个节点的执行器数
- `jenkins_node_response_time`: 单个节点的响应时间
- `jenkins_node_disk_available`: 节点剩余磁盘空间
- `jenkins_node_temp_available`: 节点剩余临时目录空间
- `jenkins_node_swap_available`: 节点可用 Swap 空间
- `jenkins_node_memory_available`: 节点可用物理内存
- `jenkins_node_swap_total`: 节点总 Swap
- `jenkins_node_memory_total`: 节点总内存

**任务 (Job) 指标:**
- `jenkins_job_duration`: 任务构建耗时
- `jenkins_job_number`: 任务构建的编号
- `jenkins_job_result_code`: 任务构建结果的状态码 (0: Success, 1: Failure, 2: Not_built, 3: Unstable, 4: Aborted)
