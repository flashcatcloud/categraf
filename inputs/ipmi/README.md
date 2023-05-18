# IPMI 插件


从[telegraf](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/ipmi_sensor/README.md) fork的ipmi_sensor ，略作改动。 采集硬件温度、风扇转速、电压、功率等信息。
- 本插件依赖ipmitool
- 采集的是ipmitool sdr的输出
  
Get bare metal metrics using the command line utility
[`ipmitool`](https://github.com/ipmitool/ipmitool).

If no servers are specified, the plugin will query the local machine sensor
stats via the following command:

```sh
ipmitool sdr
```

or with the version 2 schema:

```sh
ipmitool sdr elist
```

When one or more servers are specified, the plugin will use the following
command to collect remote host sensor stats:

```sh
ipmitool -I lan -H SERVER -U USERID -P PASSW0RD sdr
```

Any of the following parameters will be added to the aformentioned query if
they're configured:

```sh
-y hex_key -L privilege
```

## Metrics

Version 1 schema:

- ipmi_xxxx:
  - tags:
    - unit
    - host
    - server (only when retrieving stats from remote servers)
    - status_code
    - description 
  - fields:
    - xxxx

Version 2 schema:

- ipmi_xxxx:
  - tags:
    - entity_id (can help uniquify duplicate names)
    - status_code (two letter code from IPMI documentation)
    - status_desc (extended status description field)
    - unit (only on analog values)
    - host
    - server (only when retrieving stats from remote)
    - description
  - fields:
    - xxxx

### Permissions

本地采集，需要免密sudo权限

```sh
KERNEL=="ipmi*", MODE="660", GROUP="categraf采集所使用的用户组"
```

Alternatively, it is possible to use sudo. You will need the following in your
categraf config:

```toml
[[instances]]
  use_sudo = true
```

You will also need to update your sudoers file:

```bash
$ visudo
# Add the following line:
Cmnd_Alias IPMITOOL = /usr/bin/ipmitool *
UserOfCategraf  ALL=(root) NOPASSWD: IPMITOOL
Defaults!IPMITOOL !logfile, !syslog, !pam_session
```

## Example Output

### Version 1 Schema

When retrieving stats from a remote server:

```text
ipmi_cpu1_temp agent_hostname=1.2.3.4 description=40_degrees_c entity_id=3.1 server=192.168.10.173 status_code=ok unit=degrees_c 40
ipmi_cpu2_temp agent_hostname=1.2.3.4 description=42_degrees_c entity_id=3.2 server=192.168.10.173 status_code=ok unit=degrees_c 42
ipmi_pch_temp agent_hostname=1.2.3.4 description=66_degrees_c entity_id=7.1 server=192.168.10.173 status_code=ok unit=degrees_c 66
ipmi_fan3 agent_hostname=1.2.3.4 description=500_rpm entity_id=29.3 server=192.168.10.173 status_code=lnc unit=rpm 500
ipmi_fan4 agent_hostname=1.2.3.4 description=500_rpm entity_id=29.4 server=192.168.10.173 status_code=lnc unit=rpm 500
ipmi_fan5 agent_hostname=1.2.3.4 description=no_reading entity_id=29.5 server=192.168.10.173 status_code=ns status_desc=no_reading 0

```

When retrieving stats from the local machine (no server specified):

```text
ipmi_cpu1_temp agent_hostname=1.2.3.4 description=40_degrees_c status_code=ok unit=degrees_c 40
ipmi_cpu2_temp agent_hostname=1.2.3.4 description=43_degrees_c status_code=ok unit=degrees_c 43
ipmi_pch_temp agent_hostname=1.2.3.4 description=66_degrees_c status_code=ok unit=degrees_c 66
ipmi_fan3 agent_hostname=1.2.3.4 description=500_rpm status_code=nc unit=rpm 500
ipmi_fan4 agent_hostname=1.2.3.4 description=500_rpm status_code=nc unit=rpm 500
ipmi_fan5 agent_hostname=1.2.3.4 description=no_reading status_code=ns 0
```

#### Version 2 Schema

```text
ipmi_cpu1_temp agent_hostname=1.2.3.4 description=40_degrees_c entity_id=3.1 server=192.168.10.173 status_code=ok unit=degrees_c 40
ipmi_cpu2_temp agent_hostname=1.2.3.4 description=42_degrees_c entity_id=3.2 server=192.168.10.173 status_code=ok unit=degrees_c 42
ipmi_pch_temp agent_hostname=1.2.3.4 description=66_degrees_c entity_id=7.1 server=192.168.10.173 status_code=ok unit=degrees_c 66
ipmi_fan5 agent_hostname=1.2.3.4 description=no_reading entity_id=29.5 server=192.168.10.173 status_code=ns status_desc=no_reading 0
ipmi_fan6 agent_hostname=1.2.3.4 description=500_rpm entity_id=29.6 server=192.168.10.173 status_code=lnc unit=rpm 500
ipmi_fana agent_hostname=1.2.3.4 description=no_reading entity_id=29.7 server=192.168.10.173 status_code=ns status_desc=no_reading 0
```

When retrieving stats from the local machine (no server specified):

```text
ipmi_cpu1_temp agent_hostname=1.2.3.4 description=39_degrees_c entity_id=3.1 status_code=ok unit=degrees_c 39
ipmi_cpu2_temp agent_hostname=1.2.3.4 description=42_degrees_c entity_id=3.2 status_code=ok unit=degrees_c 42
ipmi_pch_temp agent_hostname=1.2.3.4 description=66_degrees_c entity_id=7.1 status_code=ok unit=degrees_c 66
ipmi_fan5 agent_hostname=1.2.3.4 description=no_reading entity_id=29.5 status_code=ns status_desc=no_reading 0
ipmi_fan6 agent_hostname=1.2.3.4 description=500_rpm entity_id=29.6 status_code=lnc unit=rpm 500
ipmi_fana agent_hostname=1.2.3.4 description=no_reading entity_id=29.7 status_code=ns status_desc=no_reading 0
```

## 示例配置
categraf 采集所在的机器需要有ipmitool 命令，如果没有，需要安装
- 本地采集
```toml
[[instances]]
## optionally specify the path to the ipmitool executable
# path = "/usr/bin/ipmitool"
##
## Setting 'use_sudo' to true will make use of sudo to run ipmitool.
## Sudo must be configured to allow the categraf user to run ipmitool
## without a password.
## 本地采集，需要免密sudo权限
    use_sudo = true
##
## optionally force session privilege level. Can be CALLBACK, USER, OPERATOR, ADMINISTRATOR
# privilege = "ADMINISTRATOR"
##
## optionally specify one or more servers via a url matching
##  [username[:password]@][protocol[(address)]]
##  e.g.
##    root:passwd@lan(127.0.0.1)
##
## if no servers are specified, local machine sensor stats will be queried
##

## Recommended: use metric 'interval' that is a multiple of 'timeout' to avoid
## gaps or overlap in pulled data
interval = "30s"

## Timeout for the ipmitool command to complete. Default is 20 seconds.
timeout = "20s"

## Schema Version: (Optional, defaults to version 1)
metric_version = 2

## Optionally provide the hex key for the IMPI connection.
# hex_key = ""

## If ipmitool should use a cache
## for me ipmitool runs about 2 to 10 times faster with cache enabled on HP G10 servers (when using ubuntu20.04)
## the cache file may not work well for you if some sensors come up late
# use_cache = false

## Path to the ipmitools cache file (defaults to OS temp dir)
## The provided path must exist and must be writable
# cache_path = ""
```
- 远程采集

```toml
[[instances]]
  ## optionally specify the path to the ipmitool executable
  # path = "/usr/bin/ipmitool"
  ##
  ## Setting 'use_sudo' to true will make use of sudo to run ipmitool.
  ## Sudo must be configured to allow the categraf user to run ipmitool
  ## without a password.
  #  use_sudo = true
  ##
  ## optionally force session privilege level. Can be CALLBACK, USER, OPERATOR, ADMINISTRATOR
  # privilege = "ADMINISTRATOR"
  ##
  ## optionally specify one or more servers via a url matching
  ##  [username[:password]@][protocol[(address)]]
  ##  e.g.
  ##    root:passwd@lan(127.0.0.1)
  ##
  ## if no servers are specified, local machine sensor stats will be queried
  ##
  ## 替换远程服务器的ip地址 用户名 密码
    servers = ["root:password@lan(192.168.10.173)"]

  ## Recommended: use metric 'interval' that is a multiple of 'timeout' to avoid
  ## gaps or overlap in pulled data
  interval = "30s"

  ## Timeout for the ipmitool command to complete. Default is 20 seconds.
  timeout = "20s"

  ## Schema Version: (Optional, defaults to version 1)
  metric_version = 1

  ## Optionally provide the hex key for the IMPI connection.
  # hex_key = ""

  ## If ipmitool should use a cache
  ## for me ipmitool runs about 2 to 10 times faster with cache enabled on HP G10 servers (when using ubuntu20.04)
  ## the cache file may not work well for you if some sensors come up late
  # use_cache = false

  ## Path to the ipmitools cache file (defaults to OS temp dir)
  ## The provided path must exist and must be writable
  # cache_path = ""
```
