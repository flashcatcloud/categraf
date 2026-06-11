# BIND 9 Input Plugin

This plugin is forked from [telegraf/bind](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/bind).

This plugin reads statistics from BIND 9's Statistics Channel (XML or JSON format), collecting metrics on DNS queries, server status, and memory context.

## Configuration

To use this plugin, you must configure the `statistics-channels` in your BIND 9 `named.conf`:

```text
statistics-channels {
  inet 127.0.0.1 port 8053 allow { 127.0.0.1; };
};
```

Then configure Categraf as follows:

```toml
[[instances]]
# URL to the BIND 9 statistics channel (XML/JSON supported)
urls = [
  "http://localhost:8053/xml/v3",
  # "http://localhost:8053/json/v1"
]

timeout = "5s"
# Set to true to collect detailed memory context metrics
gather_memory_contexts = true
# Set to true to collect metrics per view
gather_views = true
```

## Metrics

- `bind_server_*`: Global server metrics, such as total requests, queries, success, nxrrset, failure, recursion, etc.
- `bind_memory_context_*`: Internal memory usage by various BIND modules (requires `gather_memory_contexts = true`).
- `bind_view_*`: Per-view query metrics (requires `gather_views = true`).
- `bind_up`: Whether the statistics channel was reachable.
