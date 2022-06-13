# tomcat

tomcat 采集器，是读取 tomcat 的管理侧接口 `/manager/status/all` 这个接口需要鉴权。修改 `tomcat-users.xml` ，增加下面的内容：

```xml
<role rolename="admin-gui" />
<user username="tomcat" password="s3cret" roles="manager-gui" />
```

## Configuration

配置文件在 `conf/input.tomcat/tomcat.toml`

```toml
# # collect interval
# interval = 15

# Gather metrics from the Tomcat server status page.
[[instances]]
## URL of the Tomcat server status
url = "http://127.0.0.1:8080/manager/status/all?XML=true"

## HTTP Basic Auth Credentials
username = "tomcat"
password = "s3cret"

## Request timeout
# timeout = "5s"

# # interval = global.interval * interval_times
# interval_times = 1

# important! use global unique string to specify instance
# labels = { instance="192.168.1.2:8080", url="-" }

## Optional TLS Config
# use_tls = false
# tls_min_version = "1.2"
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = true
```

## 监控大盘

本 README 文件的同级目录下放置了用于 Tomcat 的 dashboard.json，大家可以导入使用。