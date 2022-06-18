# kube_state_metrics

这个插件只有一个 README，没有代码，因为 kube_state_metrics 提供了 `/metircs` 接口，所以，直接用 Categraf 的 prometheus 插件拉取其监控数据就可以了，无需单独的采集插件。

## 关于采集

kube-state-metrics 这个组件，一般只需要部署成一个单副本的 deployment 即可，使用 prometheus-operator 的话，会把 kube-state-metrics 关联一个没有 ClusterIP 的 svc，这样会给 Categraf 的采集造成困扰。

我们要修改这个 svc 的 yaml，让它自动生成 ClusterIP，这样就可以配置到 Categraf 中来抓取。

为啥 Prometheus 抓取的时候不需要 ClusterIP 呢，因为它支持 Kubernetes 的服务发现，可以查找所有的 Pod 列表，从中找到 kube-state-metrics，然后直接请求 Pod IP，这种方式可以工作，但是其实挺浪费的，个人感觉使用一个固定的 ClusterIP 会更方便。

最后，采集的时候，请为不同的 Kubernetes 集群定义一个 cluster 标签，用于区分不同的集群，该 README 同级的监控大盘，就是用了 cluster 作为大盘筛选变量。

## 指标爆炸问题

这个插件采集的数据量很大，是所有的 Kubernetes 中的对象的信息，如果集群比较大，请求 `/metrics` 甚至可能拉个几十秒种，为了提升这个拉取速度，避免指标爆炸，我们可以对 kube_state_metrics 做一些参数控制，让它只采集部分对象的数据，典型的控制手段是通过 `--resources` 参数来控制，比如我只想采集负载类型的对象：`--resources=cronjobs,jobs,daemonsets,deployments,nodes,statefulsets,pods` [完整对象列表在这里](https://github.com/prometheus-community/helm-charts/blob/56a8d0386b6e480d018033666741451f1cf35cd8/charts/kube-state-metrics/values.yaml#L160)

然后，对于这些资源类型，我们可能还想更细粒度做控制，假设有个指标：kube_deployment_spec_strategy_rollingupdate_max_surge 我们可以通过 `--metric-denylist` 来控制：

```
--metric-denylist=kube_deployment_spec_strategy_rollingupdate_max_surge
```

多个的话逗号分隔，当然，也支持正则，比如：

```
--metric-denylist=kube_deployment_spec_.*
```

这样就不会采集`kube_deployment_spec_`打头的指标了，如果集群中对象很多，比如大的集群有几千个node，几万个deployment，每个优化都很值得。

## 结语

如上是一些最佳实践，受限于个人知识水平，难免疏漏，欢迎大家提 PR 一起改进这个经验。