# Whois Input Plugin

This plugin acts as a domain probe, collecting domain registration and expiration information using the Whois protocol. All returned values are standard UTC0 Unix timestamps.

## Configuration

The core configuration involves setting the `domain` parameter for the target address you wish to monitor.

```toml
# Collect domain whois information
# interval = 86400

[[instances]]
# Target domain name. Please note that this must be a domain (e.g., "baidu.com"), NOT a URL (like "https://baidu.com").
domain = "baidu.com"
```

## Metrics

The plugin outputs the following timestamp metrics:

- `whois_domain_createddate`: Domain creation timestamp (Unix epoch).
- `whois_domain_updateddate`: Domain last update timestamp (Unix epoch).
- `whois_domain_expirationdate`: Domain expiration timestamp (Unix epoch).

All metrics include the `domain` tag for identification.

## Important Note

**Do NOT** set the `interval` too short (e.g., 10 seconds). Frequent Whois queries are unnecessary and will very likely lead to rate limiting, connection timeouts, or being IP banned by the Whois servers. Please keep the collection cycle long (e.g., once a day, `interval = 86400`).

## Dashboards

A companion basic Dashboard (`dashboard.json`) is provided in this directory. It visualizes the days remaining until domain expiration, enabling proactive alerts before crucial domains lapse.
