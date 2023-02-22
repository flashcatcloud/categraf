该插件依赖**nginx**的 **http_stub_status_module

# 应用场景
一般用于业务系统做对外或对外路由映射时使用代理服务，是运维最常见且最重要的代理工具。

# 部署场景
需要在装有nginx服务的虚拟机启用此插件。

# 前置条件
```
条件1：nginx服务需要启用http_stub_status_module模块

推荐源码编译方式安装模块，如不清楚要安装哪些模块，可参考：
cd /opt/nginx-1.20.1 && ./configure \
--prefix=/usr/share/nginx \
--sbin-path=/usr/sbin/nginx \
--modules-path=/usr/lib64/nginx/modules \
--conf-path=/etc/nginx/nginx.conf \
--error-log-path=/var/log/nginx/error.log \
--http-log-path=/var/log/nginx/access.log \
--http-client-body-temp-path=/var/lib/nginx/tmp/client_body \
--http-proxy-temp-path=/var/lib/nginx/tmp/proxy \
--http-fastcgi-temp-path=/var/lib/nginx/tmp/fastcgi \
--http-uwsgi-temp-path=/var/lib/nginx/tmp/uwsgi \
--http-scgi-temp-path=/var/lib/nginx/tmp/scgi \
--pid-path=/var/run/nginx.pid \
--lock-path=/run/lock/subsys/nginx \
--user=nginx \
--group=nginx \
--with-compat \
--with-threads \
--with-http_addition_module \
--with-http_auth_request_module \
--with-http_dav_module \
--with-http_flv_module \
--with-http_gunzip_module \
--with-http_gzip_static_module \
--with-http_mp4_module \
--with-http_random_index_module \
--with-http_realip_module \
--with-http_secure_link_module \
--with-http_slice_module \
--with-http_ssl_module \
--with-http_stub_status_module \
--with-http_sub_module \
--with-http_v2_module \
--with-mail \
--with-mail_ssl_module \
--with-stream \
--with-stream_realip_module \
--with-stream_ssl_module \
--with-stream_ssl_preread_module \
--with-select_module \
--with-poll_module \
--with-file-aio \
--with-http_xslt_module=dynamic \
--with-http_image_filter_module=dynamic \
--with-http_perl_module=dynamic \
--with-stream=dynamic \
--with-mail=dynamic \
--with-http_xslt_module=dynamic \
--add-module=/etc/nginx/third-modules/nginx_upstream_check_module \
--add-module=/etc/nginx/third-modules/ngx_devel_kit-0.3.0 \
--add-module=/etc/nginx/third-modules/lua-nginx-module-0.10.13 \
--add-module=/etc/nginx/third-modules/nginx-module-vts \
--add-module=/etc/nginx/third-modules/ngx-fancyindex-0.5.2

# 根据cpu核数
make -j2
make install

注意：第三方模块nginx_upstream_check_module lua-nginx-module nginx-module-vts 都是相关插件所必备的依赖。
```

```
条件2：nginx启用stub_status配置。

[root@aliyun conf.d]# cat nginx.domains.com.conf
server {
    listen 80;
    listen 443 ssl;
    server_name nginx.domains.com;
    include /etc/nginx/ssl_conf/domains.com.conf;

    location / {
        stub_status on;
	    include /etc/nginx/ip_whitelist.conf;
    }

    access_log /var/log/nginx/nginx.domains.com.access.log main;
    error_log /var/log/nginx/nginx.domains.com.error.log warn;
}
```

# 配置场景
```
本配置启用或数据定义如下功能：

```

# 修改nginx.toml文件配置
```

```

