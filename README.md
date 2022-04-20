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
- [x]ping
- [x]net_response
- []http_response(add cert check)
- []oom
- []promscrape
- []procstat
- [x]oracle
- []mysql
- []redis
- []nginx vts
- []tomcat
- []...
- []io.util