# Supervisor

此插件通过使用XML-RPC API收集在supervisor下运行的进程信息。

supervisor的最低测试版本为3.3.2。

## Supervisor 配置

这个插件需要在supervisor中启用HTTP服务器，同时建议在HTTP服务器上启用基本身份验证。使用基本认证时，请确保在插件的url设置中包含用户名和密码。下面是一个`inet_http_server`部分的supervisor配置示例，该配置可以与默认插件配置一起工作：

```ini
[inet_http_server]
port = 127.0.0.1:9001
username = user
password = pass
```

## 全局配置选项

除了特定于插件的配置设置外，插件还支持额外的全局和插件配置设置。这些设置用于修改指标、标签和字段或创建别名和配置排序等。

## 配置

```toml
# Gathers information about processes that running under supervisor using XML-RPC API
[[instances]]
  ## Url of supervisor's XML-RPC endpoint if basic auth enabled in supervisor http server,
  ## than you have to add credentials to url (ex. http://login:pass@localhost:9001/RPC2)
  # url = "http://login:pass@localhost:9001/RPC2"
  ## With settings below you can manage gathering additional information about processes
  ## If both of them empty, then all additional information will be collected.
  ## Currently supported supported additional metrics are: pid, rc
  # metrics_include = []
  # metrics_exclude = ["pid", "rc"]
```

注意，`url = "http://login:pass@localhost:9001/RPC2"`中的`login:pass`是用户名和密码。相关信息可以参见您的supervisor配置文件。

### 可选指标

通过在配置文件中设置`metrics_include`和`metrics_exclude`参数，用于控制哪些指标(metrics)应该被包括(`include`)或排除(`exclude`)在监控数据中。这两个配置选项为用户提供了细粒度控制，以便根据特定需要定制收集的数据。这在处理大量指标或只关心某些特定指标的情况下尤其有用。

#### metrics_include

- `metrics_include` 选项允许你指定一个指标名称列表，仅这些指标会被收集和发送。如果设置了这个选项，那么只有列表中的指标会被包含，其他所有指标都会被忽略。
- 这个选项通常用于限制数据的收集范围，以减少网络流量、存储需求或者仅仅关注一小部分重要指标。
- 格式通常是一个指标名称的数组，例如：`metrics_include = ["cpu_usage_idle", "cpu_usage_user"]`。

#### metrics_exclude

- 相反，`metrics_exclude` 选项允许你指定一个指标名称列表，这些指标将不会被收集和发送。如果设置了这个选项，那么列表中的指标会被排除，其他所有指标都会被包含。
- 这个选项用于从收集的数据中排除不感兴趣或不相关的指标，有助于减少处理和存储无用数据的负担。
- 格式同样是一个指标名称的数组，例如：`metrics_exclude = ["memory_free", "memory_cached"]`。

#### 使用注意事项

- 如果同时使用`metrics_include`和`metrics_exclude`，首先应用`metrics_include`过滤规则，然后应用`metrics_exclude`。这意味着如果一个指标在`metrics_include`中被明确包含，在`metrics_exclude`中也被明确排除，那么这个指标最终将被排除。
- 这两个配置选项的工作原理和具体可用值可能依赖于具体的插件。有的插件可能允许根据指标的某些属性或标签来进行包含或排除。
- 正确使用这两个配置选项可以显著改善Telegraf的性能和效率，特别是在资源受限的环境中或当监控系统规模较大时。

#### 示例

假设你使用Categraf监控系统性能，并使用`cpu`插件收集CPU使用情况的指标。如果你只对CPU的闲置时间和用户时间感兴趣，可以使用以下配置：

```toml
[[instances]]
  ## 仅收集CPU的闲置时间和用户使用时间的指标
  metrics_include = ["cpu_usage_idle", "cpu_usage_user"]
```

或者，如果你想收集所有CPU相关指标，但排除闲置时间和用户时间，可以使用：

```toml
[[instances]]
  ## 排除CPU的闲置时间和用户使用时间的指标
  metrics_exclude = ["cpu_usage_idle", "cpu_usage_user"]
```

通过精细控制指标的收集，你可以优化监控设置，确保只处理对你最重要的信息。

### 服务器标签

服务器标签用于标识指标源服务器。你可以选择默认使用supervisor的http端点的`host:port`，或者你可以使用在supervisor配置文件中设置的supervisor的标识字符串。

## 指标

- supervisor_processes
    - tags：
        - source（supervisor实例的主机名或IP地址）
        - port（supervisor的HTTP服务器端口号）
        - id（supervisor的标识字符串）
        - name（进程名）
        - group（进程组）
    - fields：
        - state（int，参见参考表）
        - uptime（int，秒）
        - pid（int，可选）
        - exitCode（int，可选）

- supervisor_instance
    - tags：
        - source（supervisor实例的主机名或IP地址）
        - port（supervisor的HTTP服务器端口号）
        - id（supervisor的标识字符串）
    - fields：
        - state（int，参见参考表）

### Supervisor进程状态字段参考表

| 状态码  | 状态名      | 描述                                    |
|------|----------|---------------------------------------|
| 0    | STOPPED  | 进程因停止请求停止了，或者从未启动。                    |
| 10   | STARTING | 进程因启动请求正在启动。                          |
| 20   | RUNNING  | 进程正在运行。                               |
| 30   | BACKOFF  | 进程进入STARTING状态但随后过快退出，未能移动到RUNNING状态。 |
| 40   | STOPPING | 进程因停止请求正在停止。                          |
| 100  | EXITED   | 进程已从RUNNING状态退出（预期地或意外地）。             |
| 200  | FATAL    | 无法成功启动进程。                             |
| 1000 | UNKNOWN  | 进程处于未知状态（supervisord编程错误）。            |

### Supervisor实例状态字段参考

| 状态码 | 状态名     | 描述                 |
|-----|---------|--------------------|
| 2   | FATAL   | Supervisor遇到了严重错误。 |
| 1   | RUNNING | Supervisor正在正常工作。  |
| 0   |         |                    |