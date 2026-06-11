# SNMP Zabbix Input Plugin

The `snmp_zabbix` plugin is an advanced SNMP data collection plugin that is fully compatible with Zabbix monitoring templates. Its killer feature is the ability to directly parse and execute Zabbix YAML template files. This means you can leverage the rich ecosystem of Zabbix templates without rewriting your monitoring configurations from scratch.

This is highly recommended for users migrating from Zabbix to Categraf, or for monitoring a massive array of diverse network devices (Switches, Routers, Firewalls) using existing community templates.

## Key Features

- **Zabbix Template Compatibility:** Directly uses Zabbix 6.0+ YAML format templates.
- **Low-Level Discovery (LLD):** Automatically discovers network interfaces, file systems, and other resources to dynamically create monitoring items.
- **Advanced Preprocessing:** Supports 20+ preprocessing steps including Regex, JavaScript, Custom multipliers, etc.
- **Granular Scheduling:** Supports item-level scheduling tasks.
- **Full SNMP Protocol Support:** Supports SNMPv1, v2c, and v3.

## Configuration

```toml
# Collect SNMP metrics via Zabbix Templates
# interval = 60

[[instances]]
# Target SNMP Agent address
agent = "192.168.1.1:161"

# SNMP credentials
version = "2c"
community = "public"

# The path to your Zabbix YAML templates directory
# The plugin will parse these templates to execute discovery and item polling
template_dir = "/opt/categraf/conf/zabbix_templates/"

# Specify which templates to link to this instance
templates = ["Template Net Cisco IOS SNMP"]

# Optional: Set host macros that are referenced in the Zabbix template
# [instances.macros]
# "{$SNMP_PORT}" = "161"
```

## Metrics

Because the metrics are dynamically discovered and generated based on the chosen Zabbix template, the exact metric names will vary. Typically, the plugin automatically normalizes Zabbix item keys into Prometheus-style metric names.

For example, a Zabbix item key `net.if.in[ifInOctets.1]` might be translated to `zabbix_net_if_in` with appropriate tags for the interface name and index.

## Dashboards

Since the collected metrics are entirely dependent on the specific Zabbix template you load, a universal Dashboard cannot cover all scenarios. However, we provide a generic Dashboard (`dashboard.json`) in this directory that visualizes the fundamental network interface metrics (Traffic In/Out) which are standard across almost all Zabbix Network Templates.
