# zookeeper

移植于 [dabealu/zookeeper-exporter](https://github.com/dabealu/zookeeper-exporter)，原理就是利用 Zookeper 提供的四字命令（The Four Letter Words）获取监控信息；

需要注意的是，在 zookeeper v3.4.10 以后添加了四字命令白名单，需要在 zookeeper 的配置文件 `zoo.cfg` 中新增白名单配置:
```
4lw.commands.whitelist=mntr,ruok
```

## Configuration

zookeeper 插件的配置在 `conf/input.zookeeper/zookeeper.toml` 最简单的配置如下：

```toml
[[instances]]
address = "127.0.0.1:2181"
labels = { instance="n9e-10.23.25.2:2181" }
```

如果要监控多个 zookeeper 实例，就增加 instances 即可：

```toml
[[instances]]
address = "10.23.25.2:2181"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:2181" }

[[instances]]
address = "10.23.25.3:2181"
username = ""
password = ""
labels = { instance="n9e-10.23.25.3:2181" }
```

建议通过 labels 配置附加一个 instance 标签，便于后面复用监控大盘。

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。

