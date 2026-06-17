# AppDynamics

The AppDynamics plugin fetches metrics from the AppDynamics REST API and converts them into Prometheus metrics format.

## Configuration

The plugin uses a base URL and a list of variables to dynamically construct the API queries. It parses the JSON response from AppDynamics and exports the specified values.

```toml
[[instances]]
# The base URL of the AppDynamics REST API.
url_base = "http://your-appdynamics-controller:8090/controller/rest/applications/{{.app_id}}/metric-data?metric-path={{.metric_path}}&time-range-type=BETWEEN_TIMES&start-time=$START_TIME&end-time=$END_TIME&output=JSON"

# Variables to inject into the url_base template
url_vars = [
    { app_id = "123", metric_path = "Overall Application Performance|Calls per Minute" },
    { app_id = "123", metric_path = "Overall Application Performance|Average Response Time (ms)" }
]

# Authentication credentials
username = "user@tenant"
password = "your-password"

# Filters specify which fields from the AppDynamics metric payload to extract.
# Available filters: "current", "max", "min", "count", "sum", "value".
# If empty, defaults to "current".
filters = ["current", "sum"]

# Timeout and scraping frequencies
timeout = "5s"
delay = "1m"
period = "1m"
```

## Metrics

- `up`: Indicates if the AppDynamics API endpoint was reachable and returned a valid response.
- `appdynamics_{metric_name}_{filter}`: Dynamically generated metric based on the `metric_path`. `metric_name` is derived from the last segment of the `metric_path` converted to snake_case.
  - Example: For `Overall Application Performance|Calls per Minute` with filter `current`, it will produce `appdynamics_calls_per_minute_current`.
