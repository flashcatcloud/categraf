# Greenplum Input Plugin

The Greenplum input plugin monitors the mirror node status of a Greenplum database cluster.

> Note: This plugin relies on the `gpstate` command-line tool being available in the system PATH.

## How it works

The plugin periodically executes the `gpstate -m` command in the background and parses the `Status` and `Data Status` fields from its output. Because it invokes the official Greenplum management tool directly, **Categraf must be run as a user with permissions to execute `gpstate` (e.g., `gpadmin`)**, or the environment must be configured properly.

## Configuration

```toml
# # Collect interval
# interval = 15

# There is no instance-specific configuration for this plugin. Just ensure gpstate is in the PATH.
```

## Metrics

All metrics will include `Mirror`, `Datadir`, and `Port` as tags.

- `greenplum_Status`: Node state. `1` indicates `Passive`, otherwise `0`.
- `greenplum_Data_Status`: Data synchronization status. `1` indicates `Synchronized`, otherwise `0`.

## Dashboard and Alerts

It is recommended to incorporate these status metrics into your overall Greenplum dashboard. If `greenplum_Data_Status` remains `0` for an extended period, it indicates that primary-mirror synchronization is abnormal and an alert should be triggered.
