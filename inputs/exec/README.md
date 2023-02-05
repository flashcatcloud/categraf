# exec

该插件用于给用户自定义监控脚本，监控脚本采集到监控数据之后通过相应的格式输出到stdout，categraf截获stdout内容，解析之后传给服务端，脚本的输出格式支持3种：influx、falcon、prometheus，通过 exec.toml 的 `data_format` 配置告诉 Categraf

## influx

influx 格式的内容规范：

```
mesurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
```

- 首先mesurement，表示一个类别的监控指标，比如 connections；
- mesurement后面是逗号，逗号后面是标签，如果没有标签，则mesurement后面不需要逗号
- 标签是k=v的格式，多个标签用逗号分隔，比如region=beijing,env=test
- 标签后面是空格
- 空格后面是属性字段，多个属性字段用逗号分隔
- 属性字段是字段名=值的格式，在categraf里值只能是数字

最终，mesurement和各个属性字段名称拼接成metric名字

## falcon

Open-Falcon的格式如下，举例：

```json
[
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric",
        "timestamp": 1658490609,
        "step": 60,
        "value": 1,
        "counterType": "GAUGE",
        "tags": "idc=lg,loc=beijing",
    },
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric2",
        "timestamp": 1658490609,
        "step": 60,
        "value": 2,
        "counterType": "GAUGE",
        "tags": "idc=lg,loc=beijing",
    }
]
```

timestamp、step、counterType，这三个字段在categraf处理的时候会直接忽略掉，endpoint会放到labels里上报。

## prometheus

prometheus 格式大家不陌生了，比如我这里准备一个监控脚本，输出 prometheus 的格式数据：

```shell
#!/bin/sh

echo '# HELP demo_http_requests_total Total number of http api requests'
echo '# TYPE demo_http_requests_total counter'
echo 'demo_http_requests_total{api="add_product"} 4633433'
```

其中 `#` 注释的部分，其实会被 categraf 忽略，不要也罢，prometheus 协议的数据具体的格式，请大家参考 prometheus 官方文档
