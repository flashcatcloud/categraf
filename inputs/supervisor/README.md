# Supervisor Input Plugin

This plugin gathers information about processes that
running under supervisor using XML-RPC API.

Minimum tested version of supervisor: 3.3.2

## Supervisor configuration

This plugin needs an HTTP server to be enabled in supervisor,
also it's recommended to enable basic authentication on the
HTTP server. When using basic authentication make sure to
include the username and password in the plugin's url setting.
Here is an example of the `inet_http_server` section in supervisor's
config that will work with default plugin configuration:

```ini
[inet_http_server]
port = 127.0.0.1:9001
username = user
password = pass
```

## Global configuration options

In addition to the plugin-specific configuration settings, plugins support
additional global and plugin configuration settings. These settings are used to
modify metrics, tags, and field or create aliases and configure ordering.

## Configuration

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

### Optional Metrics

By setting the `metrics_include` and `metrics_exclude` parameters in the configuration file, you can control which metrics should be included or excluded in the monitoring data. These two configuration options provide users with fine-grained control, allowing for customized data collection based on specific needs. This is especially useful when dealing with a large number of metrics or only being interested in certain specific metrics.

#### metrics_include

- The `metrics_include` option allows you to specify a list of metric names, only these metrics will be collected and sent. If this option is set, then only the metrics listed will be included, all other metrics will be ignored.
- This option is typically used to limit the scope of data collection, reducing network traffic, storage requirements, or simply focusing on a small set of important metrics.
- The format is usually an array of metric names, for example: `metrics_include = ["cpu_usage_idle", "cpu_usage_user"]`.

#### metrics_exclude

- Conversely, the `metrics_exclude` option allows you to specify a list of metric names, these metrics will not be collected and sent. If this option is set, then the metrics listed will be excluded, all other metrics will be included.
- This option is used to exclude uninteresting or irrelevant metrics from the collected data, helping to reduce the burden of processing and storing useless data.
- The format is also an array of metric names, for example: `metrics_exclude = ["memory_free", "memory_cached"]`.

#### Usage Notes

- If `metrics_include` and `metrics_exclude` are used simultaneously, the `metrics_include` filter rules are applied first, followed by `metrics_exclude`. This means that if a metric is explicitly included in `metrics_include` and also explicitly excluded in `metrics_exclude`, then the metric will ultimately be excluded.
- The workings and specific available values of these two configuration options may depend on the specific plugin. Some plugins may allow inclusion or exclusion based on certain properties or tags of the metrics.
- Properly using these two configuration options can significantly improve the performance and efficiency of Telegraf, especially in resource-constrained environments or when the monitoring system is large in scale.

#### Example

Suppose you are using Categraf to monitor system performance and are using the `cpu` plugin to collect CPU usage metrics. If you are only interested in the CPU's idle time and user time, you could use the following configuration:

```toml
[[instances]]
  ## Only collect metrics of CPU's idle time and user time
  metrics_include = ["cpu_usage_idle", "cpu_usage_user"]
```

Alternatively, if you want to collect all CPU-related metrics but exclude idle time and user time, you could use:

```toml
[[instances]]
  ## Exclude metrics of CPU's idle time and user time
  metrics_exclude = ["cpu_usage_idle", "cpu_usage_user"]
```

By finely controlling the collection of metrics, you can optimize your monitoring setup to ensure only the most important information is processed.

### Server tag

Server tag is used to identify metrics source server. You have an option
to use host:port pair of supervisor's http endpoint by default or you
can use supervisor's identification string, which is set in supervisor's
configuration file.

## Metrics

- supervisor_processes
  - Tags:
    - source (Hostname or IP address of supervisor's instance)
    - port (Port number of supervisor's HTTP server)
    - id (Supervisor's identification string)
    - name (Process name)
    - group (Process group)
  - Fields:
    - state (int, see reference)
    - uptime (int, seconds)
    - pid (int, optional)
    - exitCode (int, optional)

- supervisor_instance
  - Tags:
    - source (Hostname or IP address of supervisor's instance)
    - port (Port number of supervisor's HTTP server)
    - id (Supervisor's identification string)
  - Fields:
    - state (int, see reference)

### Supervisor process state field reference table

| Statecode | Statename | Description                                                                                              |
|-----------|-----------|----------------------------------------------------------------------------------------------------------|
| 0         | STOPPED   | The process has been stopped due to a stop request or has never been started.                            |
| 10        | STARTING  | The process is starting due to a start request.                                                          |
| 20        | RUNNING   | The process is running.                                                                                  |
| 30        | BACKOFF   | The process entered the STARTING state but subsequently exited too quickly to move to the RUNNING state. |
| 40        | STOPPING  | The process is stopping due to a stop request.                                                           |
| 100       | EXITED    | The process exited from the RUNNING state (expectedly or unexpectedly).                                  |
| 200       | FATAL     | The process could not be started successfully.                                                           |
| 1000      | UNKNOWN   | The process is in an unknown state (supervisord programming error).                                      |

### Supervisor instance state field reference

| Statecode | Statename  | Description                                    |
|-----------|------------|------------------------------------------------|
| 2         | FATAL      | Supervisor has experienced a serious error.    |
| 1         | RUNNING    | Supervisor is working normally.                |
| 0         | RESTARTING | Supervisor is in the process of restarting.    |
| -1        | SHUTDOWN   | Supervisor is in the process of shutting down. |

## Example Output

```text
supervisor_processes,group=ExampleGroup,id=supervisor,port=9001,process=ExampleProcess,source=localhost state=20i,uptime=75958i 1659786637000000000
supervisor_instance,id=supervisor,port=9001,source=localhost state=1i 1659786637000000000
```
