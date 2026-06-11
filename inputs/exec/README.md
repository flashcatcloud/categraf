# Exec Input Plugin

The Exec plugin runs user-defined monitoring scripts or programs, captures the data output to standard output (stdout), parses it, and sends it to the server.
This is one of Categraf's most flexible plugins, making it ideal for highly customized business monitoring scenarios that are not covered by the official plugin library.

## Output Formats

The executed script must print the monitoring data to stdout in one of the following 3 supported formats (configured via `data_format`):

### 1. influx
```text
measurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
```
- Measurement and tags are separated by a comma.
- Tags are separated by commas.
- A **space** separates the tags section and the fields section.
- The final metric name is usually a combination of the measurement and the field name.

### 2. prometheus
Directly output the standard Prometheus Exposition format:
```text
# HELP demo_http_requests_total Total number of http api requests
# TYPE demo_http_requests_total counter
demo_http_requests_total{api="add_product"} 4633433
```
Lines starting with `#` are ignored by Categraf.

### 3. falcon
Open-Falcon JSON format:
```json
[
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric",
        "value": 1,
        "tags": "idc=lg,loc=beijing"
    }
]
```
Fields like `timestamp`, `step`, and `counterType` are ignored. Categraf will assign the timestamp upon scraping.

## Configuration

```toml
[[instances]]
# # Commands or script paths to execute. Shell globs are supported.
commands = [
     "/opt/categraf/scripts/*/collect_*.sh",
     "/opt/categraf/scripts/*/collect_*.py"
]

# # Timeout for script execution to prevent zombie processes.
# timeout = 5

# # Format to parse the stdout data. Options: influx, prometheus, falcon
data_format = "influx"
```

## Metrics and Dashboards

Since the Exec plugin collects whatever metrics the user's scripts generate, there is no fixed list of metrics and no unified dashboard.
You should create your own dashboards and alert rules in Nightingale or Grafana based on the metric names output by your specific scripts.
