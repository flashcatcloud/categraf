# Prometheus Input Plugin

The purpose of this plugin is to generically scrape data exposed via HTTP `/metrics` endpoints (i.e., Prometheus format) and report it to the backend.
As the cloud-native ecosystem expands, an increasing number of open-source components and business applications natively embed a Prometheus SDK to expose monitoring data. You can easily scrape them using this plugin.

*(Note: Due to naming conventions and feature evolution, some of this functionality may also be handled by the `openmetrics` plugin, and their configuration logic is largely similar).*

## Configuration

It supports scraping from a static list of URLs as well as dynamic scraping via Consul service discovery.

```toml
# Scrape generic Prometheus endpoints
# interval = 15

[[instances]]
# Example of static URL configuration
urls = ["http://localhost:9100/metrics", "http://localhost:9104/metrics"]

# Metric Label Extraction Control
# By default, Categraf attaches the scraped URL as the `instance` label to every metric.
# You can customize the preserved portion using `url_label_value` (supports Go Template syntax).
# url_label_key = "instance"
# url_label_value = "{{.Host}}"
```

### Advanced URL Label Configuration

`url_label_value` supports various Go Template variables:
`{{.Scheme}}`, `{{.Host}}`, `{{.Hostname}}`, `{{.Port}}`, `{{.Path}}`, `{{.Query}}`, `{{.Fragment}}`

If you only want the hostname and port:
```toml
url_label_value = "{{.Host}}"
```

## Metrics

The collected metric names and Labels will **100% faithfully preserve** the raw data returned by the target `/metrics` endpoint.

## Dashboards

Because this plugin is a generic data scraping tool, the collected metrics depend entirely on the target service being scraped. Therefore, there is no single fixed Dashboard that fits all use cases.
When configuring panels in Grafana or Nightingale, please customize your charts based on the specific metric names of your business service. The Dashboard provided in this directory is a text-based guidance panel.
