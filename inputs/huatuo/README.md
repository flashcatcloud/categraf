# Huatuo Input Plugin

The Huatuo plugin for Categraf serves two purposes:
1. **Sidecar Mode**: Manages the lifecycle of a local `huatuo-bamai` process (Sidecar mode), including installation, configuration management, and process keep-alive.
2. **Remote Mode**: Scrapes metrics from an existing remote or local `huatuo` instance.

## Configuration

### Sidecar Mode

In this mode, Categraf will:
1. Check for `huatuo-bamai` binary in `install_path`.
2. If missing, look for `huatuo_tarball` and unpack it.
3. Read `huatuo-bamai.conf` (TOML) from the install directory.
4. Apply `config_overwrites` to the configuration and save it.
5. Launch `huatuo-bamai` and monitor it.
6. Automatically discover the metrics port from `huatuo-bamai.conf` (field `APIServer.TCPAddr`) and scrape metrics.

```toml
[[instances]]
# Path to the directory where huatuo should be installed/found.
install_path = "./huatuo" 

# (Optional) Path to the huatuo tarball to install if binary is missing.
# If using the specialized Categraf release, this is usually "embedded/huatuo.tar.gz".
huatuo_tarball = "embedded/huatuo.tar.gz"

# Overwrite specific configurations in huatuo-bamai.conf
[instances.config_overwrites]
"Storage.ES.Address" = "http://127.0.0.1:9200"
"Region" = "beijing"
"EventTracing.Softirq.DisabledThreshold" = 20000000
```

### Remote Mode

In this mode, Categraf only scrapes metrics.

```toml
[[instances]]
# URL to scrape metrics from.
url = "http://192.168.1.100:19704/metrics"

# install_path MUST be empty or omitted.
# install_path = ""
```
