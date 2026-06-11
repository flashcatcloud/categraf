# Google Cloud Monitoring Input Plugin

This plugin pulls cloud resource monitoring metrics from the Google Cloud Platform (GCP) Cloud Monitoring API (formerly Stackdriver).

## Prerequisites

Before using this plugin, ensure that the provided GCP Service Account credentials have the following permission:
- `monitoring.read` (Monitoring Viewer)

## Configuration

```toml
# Scrape interval. Since calling cloud provider APIs may incur costs and hit rate limits, it's recommended to set >= 60 seconds.
interval = 60

[[instances]]
# # Your GCP Project ID
project_id = "your-project-id"

# # Absolute path to your service account credentials JSON file
credentials_file = "/path/to/your/key.json"

# # Or configure the JSON string directly here
# credentials_json = "{...}"

# # Data delay time: Actual query End Time = Now - delay (prevents fetching empty data due to GCP write delays)
# delay = "2m"

# # Time window span for the query: Start Time = Now - delay - period
# period = "1m"

# # Metric filter (If left empty, it will default to fetching most metrics in the project, which can be very expensive)
# filter = "metric.type=\"compute.googleapis.com/instance/cpu/utilization\" AND resource.labels.zone=\"asia-northeast1-a\""

# # HTTP request timeout
# timeout = "5s"

# # Cache TTL for GCP metric metadata (effective when filter is empty, avoids frequent metadata requests)
# cache_ttl = "1h"

# # Extract the GCE (Google Compute Engine) instance_name as an alias and append it as a new Tag to the metrics
# gce_host_tag = "gce_host"

# # Maximum concurrent requests (default is 30, valid range is 1 to 100)
# request_inflight = 30

# # Force setting a larger concurrent request limit (only if you know what you are doing and won't hit GCP quotas)
# force_request_inflight = 200
```

## Metrics and Dashboards

Because this plugin is essentially a proxy for the Google Cloud Monitoring API, the collected metrics depend entirely on your `filter` rules and the GCP services you use. Therefore:
- Metric names will be automatically mapped from GCP's Metric Types.
- There is no single predefined dashboard. You will need to build custom dashboards in your monitoring system tailored to your specific workloads (e.g., GCE, Cloud SQL).
