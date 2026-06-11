# LDAP 采集插件

该插件通过查询 LDAP 服务器的监控后端 (`cn=Monitor`) 来采集指标数据。
目前，此插件支持采集 **OpenLDAP** 和 **389ds** 两种 LDAP 服务器。

在使用此插件之前，您**必须**在您的 LDAP 服务器上开启相应的监控后端或监控插件。
详细步骤可参考 [OpenLDAP Monitor 说明](https://www.openldap.org/devel/admin/monitoringslapd.html) 或 389ds 的相关文档。

## 配置说明

```toml
# 采集 LDAP 监控指标
[[instances]]
# LDAP 服务器的连接地址和端口
server = "localhost"
port = 389

# 是否使用 SSL/TLS 加密
# insecure_skip_verify = false
# starttls = false

# LDAP 绑定的账户名与密码 (需具有读取 cn=Monitor 树的权限)
# bind_dn = ""
# bind_password = ""
```

## 采集指标

根据所连接的 LDAP 服务器的底层实现（方言 dialect），插件会生成不同命名的指标。

### Tags
所有的指标都会带上以下两个默认标签：
- `server`: 连接的服务器名称或 IP
- `port`: 连接的端口

### OpenLDAP 指标
前缀通常为 `openldap_`，常见的有：
- `openldap_active_threads`: 活跃线程数
- `openldap_total_connections`: 累计建立的连接总数
- `openldap_current_connections`: 当前并发的连接数
- `openldap_bytes_statistics`: 字节统计
- `openldap_bind_operations_completed`: 成功的绑定操作数
- `openldap_search_operations_completed`: 成功的查询操作数
- `openldap_uptime_time`: 正常运行时间 (秒)

### 389ds 指标
前缀通常为 `389ds_`，常见的有：
- `389ds_current_connections`: 当前连接数
- `389ds_threads`: 当前线程数
- `389ds_operations_completed`: 完成的操作总数
- `389ds_search_operations`: 查询操作数
- `389ds_errors`: 错误数
- `389ds_bytes_sent`: 发送的字节数
