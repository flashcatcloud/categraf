# LDAP Input Plugin

This plugin gathers metrics from LDAP servers' monitoring (`cn=Monitor`) backend.
Currently, this plugin supports [OpenLDAP](https://www.openldap.org/) and [389ds](https://www.port389.org/) servers.

To use this plugin, you **must** enable the monitoring backend/plugin of your LDAP server.
See [OpenLDAP Monitoring](https://www.openldap.org/devel/admin/monitoringslapd.html) or 389ds documentation for details.

## Configuration

```toml
# Collect LDAP monitoring metrics
[[instances]]
# LDAP server host and port
server = "localhost"
port = 389

# SSL/TLS options
# insecure_skip_verify = false
# starttls = false

# Bind DN and password (must have read access to the cn=Monitor tree)
# bind_dn = ""
# bind_password = ""
```

## Metrics

Depending on the server dialect, different metrics are produced.

### Tags
All metrics will be tagged with the following:
- `server`: Server name or IP
- `port`: Port used for connecting

### OpenLDAP Metrics
Metrics start with `openldap_`, such as:
- `openldap_active_threads`
- `openldap_total_connections`
- `openldap_current_connections`
- `openldap_bytes_statistics`
- `openldap_bind_operations_completed`
- `openldap_search_operations_completed`
- `openldap_uptime_time`

### 389ds Metrics
Metrics start with `389ds_`, such as:
- `389ds_current_connections`
- `389ds_threads`
- `389ds_operations_completed`
- `389ds_search_operations`
- `389ds_errors`
- `389ds_bytes_sent`
