# cAdvisor Input Plugin

The cAdvisor input plugin collects metrics from cAdvisor. If it is collected via `kubelet`, it can optionally append pod labels and annotations.

## Configuration

```toml
# # collect interval
# interval = 15

[[instances]]
# Specify the kubelet IP and port
url = "https://1.2.3.4:10250/metrics/cadvisor"
# If the path is empty, it will be automatically appended as /metrics/cadvisor
# url = "https://1.2.3.4:10250"

# If collecting via kubelet, you can append pod labels and annotations
type = "kubelet"

# If collecting directly from cAdvisor, set type to "cadvisor"
#url = "http://1.2.3.4:8080/metrics"
#type = "cadvisor"

# Usage of url_label_key and url_label_value is explained below
url_label_key = "instance"
url_label_value = "{{.Host}}"

# Authentication token or token file
#bearer_token_string = "eyJhblonglongXXX.eyJplonglongYYY.oQsXlonglongZ-Z-Z"
bearer_token_file = "/path/to/token/file"

# Label keys to ignore
ignore_label_keys = ["id","name", "container_label*"]
# Label keys to explicitly choose. It is recommended to leave this empty to collect all labels.
# This takes precedence over ignore_label_keys.
# When this is not ["*"], include "pod" and "namespace" if you need pod labels or annotations.
#choose_label_keys = ["*"]

timeout = "3s"

# # Optional TLS Config
# # Set use_tls to true if you want to skip self-signed certificates
use_tls = true
# tls_min_version = "1.2"
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
insecure_skip_verify = true
```

## `url_label_key` and `url_label_value` Usage

```toml
# Extract the Host part from the URL and put it into the instance label
# Assuming url = https://1.2.3.4:10250/metrics/cadvisor
# The final appended label will be instance=1.2.3.4:10250

url_label_key = "instance" 
url_label_value = "{{.Host}}"
```

If you want to include both the scheme and the path, you can format it like this:

```toml
url_label_value = "{{.Scheme}}://{{.Host}}{{.Path}}"
```

The related variables are generated using the URL template fields:

| variable | value |
|---|---|
| `{{.Scheme}}` | http |
| `{{.Host}}` | 1.2.3.4:8080 |
| `{{.Hostname}}` | 1.2.3.4 |
| `{{.Port}}` | 8080 |
| `{{.Path}}` | /search |
| `{{.Query}}` | q=keyword |
| `{{.Fragment}}` | results |
