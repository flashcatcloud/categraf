# nsq
forked from [telegraf/nsq](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/nsq/nsq.go)
## Configuration
- 配置文件，[参考示例](../../conf/input.nsq/nsq.toml)

## 指标列表
### nsq_client类
ready_count     可消费消息数
inflight_count  正在处理消息数
message_count   消息总数
finish_count    完成统计
requeue_count   重新排队消息数

### nsq_channel类
depth    当前的积压量
backend_depth   消息缓冲队列积压量
inflight_count  正在处理消息数
deferred_count  延迟消息数
message_count   消息总数
requeue_count   重新排队消息数
timeout_count   超时消息数
client_count    客户端数量

### nsq_topic类
depth    消息队列积压量
backend_depth  消息缓冲队列积压量
message_count   消息总数
channel_count   消费者总数