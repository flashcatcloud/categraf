# elasticsearch

forked from telegraf/elasticsearch

## 改动

- 不再处理json中的数组类型
- 修改一些不合法的metric名称
- 配置去掉cluster_stats_only_from_master
- 调整默认配置，不采集每个索引的监控数据，nodestats不采集http数据

## 监控大盘

在该 README 文件同级目录下的 dashboard.json 可以直接导入夜莺使用