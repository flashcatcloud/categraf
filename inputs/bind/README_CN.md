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

- `bind_counter_*`: BIND 返回的各类计数器，会附带 `type`、`url`、`source`、`port` 等标签。
- `bind_memory_*`: BIND 内存汇总指标，例如 `bind_memory_total_use`、`bind_memory_in_use`。
- `bind_memory_context_*`: BIND 内部各模块的内存使用量，例如 `bind_memory_context_total`、`bind_memory_context_in_use`（需开启 `gather_memory_contexts`）。

开启 `gather_views = true` 后，按 DNS View 统计的计数器也会以 `bind_counter_*` 上报，并额外附带 `view` 标签。
