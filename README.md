# Categraf

Categraf is a monitoring agent for nightingale/prometheus/m3db/victoriametrics/thanos/influxdb/tdengine.

## QuickStart

[QuickStart](https://www.gitlink.org.cn/flashcat/categraf/wiki)

## Releases

[Releases](https://www.gitlink.org.cn/flashcat/categraf/releases)

## Build

```shell
# export GO111MODULE=on
# export GOPROXY=https://goproxy.cn
go build
```

## Deploy categraf as daemonset

```shell
edit k8s/categraf.yaml, replace NSERVER_SERVICE_WITH_PORT with service ip:port of nserver in your cluster, replace CATEGRAF_NAMESPACE with namespace value, then run:

kubectl apply -n monitoring -f ks8/categraf.yaml
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

- [x] [system](inputs/system)
- [x] [kernel](inputs/kernel)
- [x] [kernel_vmstat](inputs/kernel_vmstat)
- [x] [linux_sysctl_fs](inputs/linux_sysctl_fs)
- [x] [cpu](inputs/cpu)
- [x] [mem](inputs/mem)
- [x] [net](inputs/net)
- [x] [netstat](inputs/netstat)
- [x] [disk](inputs/disk)
- [x] [diskio](inputs/diskio)
- [x] [ntp](inputs/ntp)
- [x] [processes](inputs/processes)
- [x] [exec](inputs/exec)
- [x] [ping](inputs/ping)
- [x] [http_response](inputs/http_response)
- [x] [net_response](inputs/net_response)
- [x] [procstat](inputs/procstat)
- [x] [mysql](inputs/mysql)
- [x] [redis](inputs/redis)
- [x] [oracle](inputs/oracle)
- [x] [rabbitmq](inputs/rabbitmq)
- [x] [prometheus](inputs/prometheus)
- [x] [tomcat](inputs/tomcat)
- [x] [nvidia_smi](inputs/nvidia_smi)
- [x] [nginx_upstream_check](inputs/nginx_upstream_check)
- [x] [kubernetes(read metrics from kubelet api)](inputs/kubernetes)
- [ ] k8s solution
- [x] [nginx vts](inputs/nginx_vts)
- [ ] mongodb
- [ ] rocketmq
- [ ] activemq
- [ ] kafka
- [ ] elasticsearch
- [ ] prometheus discovery
- [x] windows
- [ ] mssql
- [ ] iis
- [ ] weblogic
- [ ] was
- [ ] hadoop
- [ ] ad
- [ ] zookeeper
- [ ] statsd
- [ ] snmp
- [x] [switch_legacy](inputs/switch_legacy)
- [ ] ipmi
- [ ] smartctl
- [ ] logging
- [ ] trace

## FAQ

[FAQ](https://www.gitlink.org.cn/flashcat/categraf/wiki)

## Thanks

Categraf is developed on the basis of Telegraf and Exporters. Thanks to the great open source community.
