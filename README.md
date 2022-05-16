# Categraf

Categraf is a monitoring agent for nightingale.

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

## TODO

- [ ] k8s solution
- [ ] nginx vts
- [ ] mongodb
- [ ] rabbitmq dashboard
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
- [ ] io.util

## Thanks

Categraf is developed on the basis of Telegraf and Exporters. Thanks to the great open source community.
