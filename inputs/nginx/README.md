# nginx

nginx 监控采集插件，由telegraf改造而来。

该插件依赖**nginx**的 **http_stub_status_module**

在**nginx**中添加如下配置：
```nginx
server{
    listen 8000;
    server_name _;
    
    location /nginx_status {
        stub_status on;
        access_log off;
    }
}
```
## Configuration

请参考配置[示例](../../conf/input.nginx/nginx.toml)

## 监控大盘和告警规则

待更新...