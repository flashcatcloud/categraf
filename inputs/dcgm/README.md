# DCGM Input Plugin

This plugin collects hardware monitoring metrics for NVIDIA GPUs by integrating the core logic of the official NVIDIA `dcgm-exporter`. Using the Data Center GPU Manager (DCGM), the plugin gathers detailed hardware statistics including GPU temperature, power usage, frame buffer (memory) usage, GPU utilization, and XID errors.

> Note: This plugin is only included when Categraf is compiled with the `dcgm` build tag (e.g., `go build -tags "dcgm"`).

## Configuration

```toml
[[instances]]
  # Path to the DCGM collectors CSV configuration file, which defines the FieldIDs to monitor.
  # Example: "/etc/categraf/dcgm/default-counters.csv"
  collectors = "/etc/categraf/dcgm/default-counters.csv"

  # Whether Categraf is running in a Kubernetes environment
  kubernetes = false
  # Type of GPU ID resolution in k8s (e.g., "uid")
  kubernetes-gpu-id-type = "uid"

  # Device selection string, e.g., "f" (flex, default), "g" (all GPUs), "i" (GPU instances)
  devices = "f"

  # Whether to use fake GPUs (useful for testing and development)
  fake-gpus = false

  # Optional: Connect to a remote hostengine
  # remote-hostengine-info = "localhost:5555"

  # You can declare the collector CSV file inline directly in the config
  # [instances.collector_files]
  # "/etc/categraf/dcgm/default-counters.csv" = """
  # DCGM_FI_DEV_GPU_TEMP, gauge, GPU temperature (in C)
  # DCGM_FI_DEV_POWER_USAGE, gauge, Power draw (in W).
  # """
```

## Metrics

All metrics will be tagged with identifiers such as `gpu`, `UUID`, `device`. Common metrics include:

- `DCGM_FI_DEV_GPU_TEMP`: GPU temperature (in Celsius)
- `DCGM_FI_DEV_POWER_USAGE`: Real-time power draw (in Watts)
- `DCGM_FI_DEV_GPU_UTIL`: GPU compute utilization (%)
- `DCGM_FI_DEV_MEM_COPY_UTIL`: Frame buffer read/write utilization (%)
- `DCGM_FI_DEV_FB_USED`: Frame buffer memory used (MB)
- `DCGM_FI_DEV_FB_FREE`: Frame buffer memory free (MB)
- `DCGM_FI_DEV_XID_ERRORS`: Number of XID hardware/driver errors encountered by the GPU

## Dashboard

A standard DCGM Dashboard is provided as a reference, covering essential monitoring panels like GPU Utilization, Power Usage, Frame Buffer Memory, and Temperature.
