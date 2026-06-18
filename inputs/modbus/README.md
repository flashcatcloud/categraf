# Modbus Plugin

This plugin gathers data from Modbus devices via TCP or RTU.

> **Fork Notice**: This plugin is ported and modified from [InfluxData Telegraf's Modbus plugin](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/modbus) (Upstream commit: `65e95e6fd4e7e9ca1636102d8aebe7870694ebad`).

## Configuration

Please see the example configuration file at `conf/input.modbus/modbus.toml` for available options.

### Compatibility with Telegraf

This plugin retains high compatibility with Telegraf's `[[inputs.modbus]]` configurations:
- All three configuration styles (`register`, `request`, `metric`) are fully supported.
- Retains all Telegraf workarounds like `close_connection_after_gather` and `pause_between_requests`.

### Categraf Label Overrides

The tag override hierarchy is strictly:
`Built-in Labels (name/type/slave_id)` < `Modbus Custom Tags` < `Categraf Instance Labels`

Note: Categraf's Instance Labels will take highest precedence (even allowing label deletion with `"-"`). Global Labels (`[global.labels]`) and `agent_hostname` are only supplemented if they do not exist, and will not override existing labels.

## Metrics

The measurement name is set to `modbus` by default. Under the `metric` configuration type, if a custom `measurement` name is provided, the resulting metric name in Categraf will be `{measurement}_{field_name}`.

### Example Metrics

```
hvac_temperature 25.5
hvac_compressor_status 1
```
