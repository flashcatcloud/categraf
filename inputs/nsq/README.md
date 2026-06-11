# NSQ Input Plugin

This plugin collects metrics from [NSQ](https://nsq.io/), a realtime distributed messaging platform. 
It is forked from [telegraf/nsq](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/nsq/nsq.go).

## Configuration

For configuration options, please refer to the [example configuration](../../conf/input.nsq/nsq.toml).

```toml
# Collect NSQ metrics
# interval = 15

[[instances]]
# endpoints array of NSQd or NSQlookupd HTTP API URLs
endpoints = ["http://localhost:4151"]
```

## Metrics

### nsq_client Metrics
- `ready_count`: Number of messages the client is ready to receive
- `inflight_count`: Number of messages currently in-flight
- `message_count`: Total number of messages received
- `finish_count`: Total number of finished (FIN) messages
- `requeue_count`: Total number of requeued (REQ) messages

### nsq_channel Metrics
- `depth`: Total number of messages in the channel (memory + disk backlog)
- `backend_depth`: Number of messages in the disk queue
- `inflight_count`: Number of in-flight messages (delivered but not yet FIN/REQ/TIMEOUT)
- `deferred_count`: Number of deferred (delayed) messages
- `message_count`: Total number of messages processed since startup
- `requeue_count`: Total number of requeued messages
- `timeout_count`: Total number of timed-out messages
- `client_count`: Number of clients connected to this channel

### nsq_topic Metrics
- `depth`: Total number of messages in the topic queue
- `backend_depth`: Number of messages in the topic disk queue
- `message_count`: Total number of messages received
- `channel_count`: Total number of channels connected to the topic

## Dashboards

A basic Dashboard (`dashboard.json`) is provided in this directory to monitor NSQ Server status, topic depth, channel depth, and message throughput.
