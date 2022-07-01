# Categraf
![Release](https://github.com/flashcatcloud/categraf/workflows/Release/badge.svg)
[![Powered By Flashcat](https://img.shields.io/badge/Powered%20By-Flashcat-red)](https://flashcat.cloud/)

Categraf is a monitoring agent for nightingale / prometheus / m3db / victoriametrics / thanos / influxdb / tdengine.

[![dockeri.co](https://dockeri.co/image/flashcatcloud/categraf)](https://hub.docker.com/r/flashcatcloud/categraf/)

## Links

- [QuickStart](https://www.gitlink.org.cn/flashcat/categraf/wiki/QuickStart)
- [FAQ](https://www.gitlink.org.cn/flashcat/categraf/wiki/FAQ)
- [Github Releases](https://github.com/flashcatcloud/categraf/releases)
- [Gitlink Releases](https://www.gitlink.org.cn/flashcat/categraf/releases)

## Build

```shell
# export GO111MODULE=on
# export GOPROXY=https://goproxy.cn
go build
```

## Pack

```shell
tar zcvf categraf.tar.gz categraf conf
```


## Run

```shell
# test mode: just print metrics to stdout
./categraf --test

# test system and mem plugins
./categraf --test --inputs system:mem

# print usage message
./categraf --help

# run
./categraf

# run with specified config directory
./categraf --configs /path/to/conf-directory

# only enable system and mem plugins
./categraf --inputs system:mem

# use nohup to start categraf
nohup ./categraf &> stdout.log &
```


## Deploy categraf as daemonset

edit k8s/daemonset.yaml, replace NSERVER_SERVICE_WITH_PORT with service ip:port of nserver in your cluster, replace CATEGRAF_NAMESPACE with namespace value, then run:

```shell
kubectl apply -n monitoring -f ks8/daemonset.yaml
kubectl apply -n monitoring -f ks8/sidecar.yaml
```
Notice: k8s/sidecar.yaml is a demo, replace mock with your own image.


## Plugin

Click on the links to see the README of each plugin.

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
- [x] [conntrack](inputs/conntrack)
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
- [x] [kube_state_metrics](inputs/kube_state_metrics)
- [x] [nginx_vts](inputs/nginx_vts)
- [ ] mongodb
- [ ] rocketmq
- [ ] activemq
- [ ] kafka
- [x] [elasticsearch](inputs/elasticsearch)
- [x] windows
- [ ] mssql
- [ ] iis
- [ ] weblogic
- [ ] was
- [ ] hadoop
- [ ] ad
- [x] [zookeeper](inputs/zookeeper)
- [ ] statsd
- [ ] snmp
- [x] [switch_legacy](inputs/switch_legacy)
- [ ] ipmi
- [ ] smartctl
- [ ] logging
- [ ] trace


## Thanks

Categraf is developed on the basis of Telegraf and Exporters. Thanks to the great open source community.

## Community

![](doc/laqun.jpeg)
