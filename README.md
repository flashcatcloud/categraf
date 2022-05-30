# Categraf

Categraf is a monitoring agent for nightingale.

## Releases

[Releases](https://www.gitlink.org.cn/flashcat/categraf/releases)

## Build

```shell
# export GO111MODULE=on
# export GOPROXY=https://goproxy.cn
go build
```

## Test

```shell
./categraf --test

# usage:
./categraf --help
```

## Pack

```shell
tar zcvf categraf.tar.gz categraf conf
```

## Plan

- [x] system
- [x] kernel
- [x] kernel_vmstat
- [x] linux_sysctl_fs
- [x] cpu
- [x] mem
- [x] net
- [x] netstat
- [x] disk
- [x] diskio
- [x] ntp
- [x] processes
- [x] exec
- [x] ping
- [x] http_response
- [x] net_response
- [x] procstat
- [x] mysql
- [x] redis
- [x] oracle
- [x] rabbitmq
- [x] prometheus
- [x] tomcat
- [x] nvidia_smi
- [ ] k8s solution
- [ ] nginx vts
- [ ] mongodb
- [ ] rocketmq
- [ ] activemq
- [ ] kafka
- [ ] elasticsearch
- [ ] prometheus discovery
- [ ] windows
- [ ] mssql
- [ ] iis
- [ ] weblogic
- [ ] was
- [ ] hadoop
- [ ] ad
- [ ] zookeeper
- [ ] statsd
- [ ] snmp
- [ ] ipmi
- [ ] smartctl
- [ ] logging
- [ ] trace

## FAQ

[FAQ](https://www.gitlink.org.cn/flashcat/categraf/wiki)

## Thanks

Categraf is developed on the basis of Telegraf and Exporters. Thanks to the great open source community.
