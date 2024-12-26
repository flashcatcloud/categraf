# rabbitmq

rabbitmq 监控采集插件，fork 自：[telegraf/rabbitmq](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/rabbitmq) 。不过，这个插件用处不大了，因为从 rabbitmq 3.8 版本开始，就内置了 prometheus 的支持，即，如果 rabbitmq 启用了 prometheus，可以直接暴露 metrics 接口，Categraf 从这个 metrics 接口拉取数据即可

rabbitmq 启用 prometheus 插件：

```shell
rabbitmq-plugins enable rabbitmq_prometheus
```

启用成功的话，rabbitmq 默认会在 15692 端口起监听，访问 [http://localhost:15692/metrics](http://localhost:15692/metrics) 即可看到符合 prometheus 协议的监控数据。

于是，使用 Categraf 的 prometheus 插件，来抓取即可，无需使用 rabbitmq 这个插件了。

本 README 文件的同级目录，放置了一个 dashboard.json 就是为 rabbitmq 3.8 以上版本准备的，可以导入夜莺使用。