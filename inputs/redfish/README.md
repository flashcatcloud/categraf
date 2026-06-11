# Redfish Input Plugin

This plugin collects hardware sensor and status metrics from Out-of-Band (OOB) management interfaces that support the Redfish protocol (such as Dell iDRAC, HPE iLO, Lenovo XClarity, etc.).
Compared to the legacy IPMI protocol, Redfish provides richer hardware metrics formatted in JSON via modern HTTP/RESTful APIs.

## Configuration

```toml
# Collect Redfish hardware status metrics
# interval = 60

[[instances]]
# Configure connection addresses, accounts, and passwords for Redfish
# [[instances.addresses]]
# url = "https://10.0.0.1"
# username = "admin"
# password = "password"
# (Redfish often uses self-signed certificates, so you may want to skip TLS verification)
# insecure_skip_verify = true

# ================================
# Examples for defining metric collection paths (Sets/Metrics)
# The plugin parses specific numeric metrics based on the defined URN and JSON Paths
# ================================

[[instances.sets]]
urn = "/redfish/v1/Chassis/System.Embedded.1/Thermal"
prefix = "thermal_"
[[instances.sets.metrics]]
name = "temperature"
path = "Temperatures.#.ReadingCelsius"
[[instances.sets.metrics.tags]]
name = "name"
path = "Temperatures.#.Name"

[[instances.sets]]
urn = "/redfish/v1/Chassis/System.Embedded.1/Power"
prefix = "power_"
[[instances.sets.metrics]]
name = "consumed_watts"
path = "PowerControl.#.PowerConsumedWatts"
```

## Metrics

The metrics gathered by this plugin are completely dynamic, determined by the `sets` and `metrics` (parsed using JSON Path) in the configuration file. Typically, we monitor:

- **Temperatures**: `redfish_thermal_temperature` (Celsius readings of various sensors)
- **Power**: `redfish_power_consumed_watts` (Current system power consumption)
- **Fans**: Fan speeds (RPM or percentage)
- **Disks**: Health statuses of physical disks and logical volumes
- **Power Supplies**: Operational statuses of redundant power supply modules

By default, all metrics carry labels like the Redfish request URL, and you can also use `tags` to extract name fields from the JSON as Labels (e.g., extracting `Temperatures.#.Name` as the sensor name).

## Dashboards

A basic companion Dashboard (`dashboard.json`) is provided in this directory to quickly visualize critical hardware health indicators like server ambient temperatures and total power consumption collected via Redfish.
