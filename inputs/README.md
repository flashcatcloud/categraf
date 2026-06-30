# inputs

每个采集插件就是一个目录，大家可以点击各个目录进去查看，每个插件的使用方式，都提供了 README 和默认配置，一目了然。如果想贡献插件，可以拷贝 tpl 目录的代码，基于 tpl 做改动。

## 监控大盘

插件目录下的 `dashboard*.json`、`dashboards.json`、`*_dash.json` 等文件通常是夜莺 Dashboard；对应的 `*_grafana.json` 文件是 Grafana Dashboard，可在 Grafana 中选择 Prometheus 兼容数据源后导入使用。
