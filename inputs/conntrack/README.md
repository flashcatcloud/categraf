# conntrack

运维老鸟应该会遇到 conntrack table full 的报错吧，这个插件就是用于监控 conntrack 的情况， forked from [telegraf/conntrack](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/conntrack)

## Measurements & Fields

- conntrack
  - ip_conntrack_count (int, count): the number of entries in the conntrack table
  - ip_conntrack_max (int, size): the max capacity of the conntrack table

## 告警

```
100 * conntrack_ip_conntrack_count / conntrack_ip_conntrack_max > 0.8
100 * conntrack_nf_conntrack_count / conntrack_nf_conntrack_max > 0.8
```