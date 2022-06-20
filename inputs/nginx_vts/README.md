# nginx_vts

nginx_vts 已经支持输出 prometheus 格式的数据，所以，其实已经不需要这个采集插件了，直接用 categraf 的 prometheus 采集插件，读取 nginx_vts 的 prometheus 数据即可。

## Configuration

假设 nginx_vts 插件的访问路径是 `/vts` ，请求其 `/vts/format/prometheus` 就可以抓到 prometheus 的数据了。

## 监控大盘

这个插件暂无监控大盘，如果有人做了 vts 的监控大盘，欢迎导出大盘 JSON 配置，提 PR 到这个目录