# kafka

kafka 监控采集插件，由 [kafka-exporter](https://github.com/davidmparrott/kafka_exporter) 封装而来。

## Configuration

请参考配置[示例](../../conf/input.kafka/kafka.toml)

## 监控大盘和告警规则

同级目录下的 dashboard.json、alerts.json 可以直接导入夜莺使用。


## 开源kafka-exporter 兼容说明
categraf的exporter 封装 https://github.com/davidmparrott/kafka_exporter  (以下简称davidmparrott版本)
davidmparrott版本  fork自 https://github.com/danielqsj/kafka_exporter   (以下简称danielqsj版本)



danielqsj版本作为原始版本, github版本也相对活跃, prometheus生态使用较多
categraf kafka plugin基于davidmparrott版本, 以下配置可以对danielqsj版本做一些兼容

1. 增加metric: kafka_broker_info
   davidmparrott版本无此metric, 默认以增加, 无需配置
2. davidmparrott版本与danielqsj版本, 有以下metric名字不同:
   | davidmparrott版本  | danielqsj版本 |
   | ---- | ---- |
   | kafka_consumergroup_uncommit_offsets  | kafka_consumergroup_lag |
   | kafka_consumergroup_uncommit_offsets_sum  | kafka_consumergroup_lag_sum |
   | kafka_consumergroup_uncommitted_offsets_zookeeper | kafka_consumergroup_lag_zookeeper |

如果想使用danielqsj版本的metric, 在kafka instance 配置文件中进行如下配置:
```toml
rename_uncommit_offset_to_lag = true
```

3. davidmparrott版本 比 danielqsj版本多以下metric
   以下metric是对延迟速率进行了计算

- kafka_consumer_lag_millis
- kafka_consumer_lag_interpolation
- kafka_consumer_lag_extrapolation

计算在内存一map, 记录每个partition的offset时序数据, 时序数据的数量配置文件`max_offsets`来控制
**较大的kafka集群会建议关闭此功能**, 占用内存较多, 计算速率可以使用promeql rate进行计算.
可以在kafka instance 配置文件中进行如下配置:
```toml
disable_calculate_lag_rate = true
```

4. 增加参数配置offset_show_all, 默认为true, 采集所有consumer group. 配置为false的话仅采集connected consumer groups
   davidmparrott版本无此配置.
