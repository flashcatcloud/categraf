# BIND 9 采集插件

该插件 fork 自 [telegraf/bind](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/bind)。

此插件通过读取 BIND 9 的 XML 或 JSON 状态统计接口（Statistics Channel）来收集 DNS 查询、服务器状态以及内存等指标数据。

## 配置说明

要使用此插件，您需要在 `named.conf` 中配置统计通道 (statistics-channel)，例如：

```text
statistics-channels {
  inet 127.0.0.1 port 8053 allow { 127.0.0.1; };
};
```

然后在 Categraf 中进行如下配置：

```toml
[[instances]]
# BIND 9 状态接口地址，支持 XML/JSON
urls = [
  "http://localhost:8053/xml/v3",
  # "http://localhost:8053/json/v1"
]

timeout = "5s"
# 是否采集详细的内存上下文指标
gather_memory_contexts = true
# 是否采集视图 (views) 相关指标
gather_views = true
```

## 采集指标

- `bind_server_*`: BIND 服务器的全局请求数、查询数、成功/失败/拒绝的解析数等。
- `bind_memory_context_*`: BIND 内部各模块的内存使用量（需开启 `gather_memory_contexts`）。
- `bind_view_*`: 按 DNS View 统计的查询数据（需开启 `gather_views`）。
- `bind_up`: 目标统计接口是否可达。