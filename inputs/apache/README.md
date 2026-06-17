# Apache

This plugin collects metrics from the Apache HTTP Server by parsing the output of the `mod_status` module.

## Configuration

To use this plugin, you must enable `mod_status` in your Apache configuration and make it accessible. It is highly recommended to append `?auto` to the scrape URI to get the machine-readable output.

```toml
[[instances]]
# The URL to the server-status page.
# Ensure that '?auto' is appended to the URL.
scrape_uri = "http://localhost/server-status/?auto"

# Optional: Override the Host header
# host_override = "example.com"

# Optional: Skip TLS verification
# insecure = false

# Optional: Custom request headers
# custom_headers = {}

# Optional: Log level, one of debug, info, warn, error
# log_level = "info"
```

### Apache mod_status Setup

Enable the `mod_status` module in your `httpd.conf` or `apache2.conf`:

```apache
<Location "/server-status">
    SetHandler server-status
    Require local
</Location>
```

Restart Apache for the changes to take effect.

## Metrics

- `apache_accesses_total`: Current total accesses
- `apache_workers`: Apache worker states (busy, idle, etc.)
- `apache_scoreboard`: Number of workers in each state
- `apache_up`: Indicates whether the Apache server was reachable
- `apache_uptime_seconds_total`: Current uptime in seconds
