# Conntrack Input Plugin

This plugin monitors the connection tracking (conntrack) table on Linux servers. It is forked from `telegraf/conntrack`.

System administrators often encounter the `nf_conntrack: table full, dropping packet` error. This plugin helps you monitor the usage of the conntrack table in real-time to prevent such issues.

## Metrics

All metrics are recorded under the `conntrack` measurement:

- `conntrack_ip_conntrack_count`: The current number of entries in the conntrack table.
- `conntrack_ip_conntrack_max`: The maximum capacity of the conntrack table.
- `conntrack_nf_conntrack_count`: The current number of entries in the nf_conntrack table.
- `conntrack_nf_conntrack_max`: The maximum capacity of the nf_conntrack table.

## Alerting Recommendation

You can configure an alerting rule in your monitoring system (like Prometheus or Nightingale) to trigger an alert when the conntrack table is close to being full:

```promql
conntrack_ip_conntrack_count / conntrack_ip_conntrack_max > 0.8
conntrack_nf_conntrack_count / conntrack_nf_conntrack_max > 0.8
```
