# categraf

monitoring agent

## build

```shell
export GO111MODULE=on
export GOPROXY=https://goproxy.cn
go build
```

## pack

```shell
tar zcvf categraf.tar.gz categraf conf
```

## todo

- []ntp
- [x]exec
- []ping
- []net_response
- []http_response
- []scrape
- []procstat
- [x]oracle
- []mysql
- []redis
- []...