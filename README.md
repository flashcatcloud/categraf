## Categraf

<a href="https://github.com/flashcatcloud/categraf">
  <img src="doc/categraf.png" alt="categraf, one-stop telemetry collector" width="80" />
</a>

[![Powered By Flashcat](https://img.shields.io/badge/Powered%20By-Flashcat-blueviolet)](https://flashcat.cloud/)
[![Release](https://img.shields.io/github/v/release/flashcatcloud/categraf)](https://github.com/flashcatcloud/categraf/releases/latest)
[![Docker pulls](https://img.shields.io/docker/pulls/flashcatcloud/categraf)](https://hub.docker.com/r/flashcatcloud/categraf/)
[![Starts](https://img.shields.io/github/stars/flashcatcloud/categraf)](https://github.com/flashcatcloud/categraf/stargazers)
[![Forks](https://img.shields.io/github/forks/flashcatcloud/categraf)](https://github.com/flashcatcloud/categraf/fork)
[![Contributors](https://img.shields.io/github/contributors-anon/flashcatcloud/categraf)](https://github.com/flashcatcloud/categraf/graphs/contributors)
[!["License"](https://img.shields.io/badge/license-MIT-blue)](https://github.com/flashcatcloud/categraf/blob/main/LICENSE)

Categraf is one-stop telemetry collector for nightingale / prometheus / m3db / victoriametrics / thanos / influxdb / tdengine.


## Links

- [QuickStart](https://flashcat.cloud/blog/monitor-agent-categraf-introduction/)
- [FAQ](https://www.gitlink.org.cn/flashcat/categraf/wiki/FAQ)
- [Github Releases](https://github.com/flashcatcloud/categraf/releases)

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


## Deploy categraf as daemonset, deployment or sidecar

edit k8s/daemonset.yaml, replace NSERVER_SERVICE_WITH_PORT with service ip:port of nserver in your cluster, replace CATEGRAF_NAMESPACE with namespace value, then run:

```shell
kubectl apply -n monitoring -f k8s/daemonset.yaml # collect metrics, metrics/cadvisor of node
kubectl apply -n monitoring -f k8s/sidecar.yaml # collect service metrics
kubectl apply -n monitoring -f k8s/deployment.yaml #collect apiserver coredns etc
```
Notice: k8s/sidecar.yaml is a demo, replace mock with your own image of service.

## Scrape like prometheus
see detail [here](https://github.com/flashcatcloud/categraf/blob/main/prometheus/README.md)

## Plugin

plugin list and document: [https://github.com/flashcatcloud/categraf/tree/main/inputs](https://github.com/flashcatcloud/categraf/tree/main/inputs) 


## Thanks

Categraf is developed on the basis of Telegraf, Exporters and the OpenTelemetry. Thanks to the great open source community.

## Community

<img src="doc/laqun.jpeg" alt="categraf contact" width="240" />
