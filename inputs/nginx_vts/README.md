# nginx_vts

nginx_vts 已经支持输出 prometheus 格式的数据，所以，其实已经不需要这个采集插件了，直接用 categraf 的 prometheus 采集插件，读取 nginx_vts 的 prometheus 数据即可。

## Configuration

假设 nginx_vts 插件的访问路径是 `/vts` ，请求其 `/vts/format/prometheus` 就可以抓到 prometheus 的数据了。

## 监控大盘

https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_vts/dashboards.json
