# kafka

kafka 监控采集插件，封装kafka-exporter（https://github.com/davidmparrott/kafka_exporter）而来

## Configuration

```toml
# # collect interval
# interval = 15

# 要监控 MySQL，首先要给出要监控的MySQL的连接地址、用户名、密码
[[instances]]

```

## 监控大盘和告警规则

本 README 的同级目录，大家可以看到 dashboard.json 就是监控大盘，导入夜莺就可以使用，alerts.json 是告警规则，也是导入夜莺就可以使用。