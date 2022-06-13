# nginx_upstream_check

该采集插件是读取 [nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module) 的状态输出。[nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module) 可以周期性检查 upstream 中的各个 server 是否存活，如果检查失败，就会标记为 `down`，如果检查成功，就标记为 `up`。

由于 TSDB 通常无法处理字符串，所以 Categraf 会做转换，将 `down` 转换为 2， `up` 转换为 1，其他状态转换为 0，使用 `nginx_upstream_check_status_code` 这个指标来表示，所以，我们可能需要这样的告警规则：

```
nginx_upstream_check_status_code != 1
```

## Configuration

配置文件在 `conf/input.nginx_upstream_check/nginx_upstream_check.toml`

```toml
# # collect interval
# interval = 15

[[instances]]
# 这个配置最关键，是要给出获取 status 信息的接口地址
targets = [
    # "http://127.0.0.1/status?format=json",
    # "http://10.2.3.56/status?format=json"
]

# 标签这个配置请注意
# 如果 Categraf 和 Nginx 是在一台机器上，target 可能配置的是 127.0.0.1
# 如果 Nginx 有多台机器，每台机器都有 Categraf 来采集本机的 Nginx 的 Status 信息
# 可能会导致时序数据标签相同，不易区分，当然，Categraf 会自带 ident 标签，该标签标识本机机器名
# 如果大家觉得 ident 标签不够用，可以用下面 labels 配置，附加 instance、region 之类的标签
# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Set http_proxy (categraf uses the system wide proxy settings if it's is not set)
# http_proxy = "http://localhost:8888"

## Interface to use when dialing an address
# interface = "eth0"

## HTTP Request Method
# method = "GET"

## Set timeout (default 5 seconds)
# timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
# username = "username"
# password = "pa$$word"

## Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```