# kubernetes

forked from telegraf/kubernetes

增加了一些控制开关：

`gather_system_container_metrics = true`

是否采集静态容器，比如 kubelet 一般就是静态容器，非业务容器

`gather_node_metrics = false`

是否采集 node 层面的指标，机器层面的指标其实 categraf 来采集了，这里理论上不需要再采集了

`gather_pod_container_metrics = true`

是否采集 Pod 中的容器的指标，这些 Pod 一般是业务容器

`gather_pod_volume_metrics = true`

是否采集 Pod 的数据卷的指标

`gather_pod_network_metrics = true`

是否采集 Pod 的网络监控数据
