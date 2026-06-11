
forked  from  [telegraf](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/snmp_trap)
# SNMP Trap Input Plugin

The SNMP Trap plugin is a service input plugin that receives SNMP
notifications (traps and inform requests).

Notifications are received on plain UDP. The port to listen is
configurable.

## Note about Paths

Path is a global variable, separate snmp instances will append the specified
path onto the global path variable

## Global configuration options <!-- @/docs/includes/plugin_config.md -->

In addition to the plugin-specific configuration settings, plugins support
additional global and plugin configuration settings. These settings are used to
modify metrics, tags, and field or create aliases and configure ordering, etc.
See the [CONFIGURATION.md][CONFIGURATION.md] for more details.

[CONFIGURATION.md]: ../../../docs/CONFIGURATION.md#plugins

## Secret-store support

This plugin supports secrets from secret-stores for the `sec_name`,
`auth_password` and `priv_password` option.
See the [secret-store documentation][SECRETSTORE] for more details on how
to use them.

[SECRETSTORE]: ../../../docs/CONFIGURATION.md#secret-store-secrets

## SNMP backend: gosmi and netsnmp

Telegraf has two backends to translate SNMP objects. By default, Telegraf will
use `netsnmp`, however, this option is deprecated and it is encouraged that
users migrate to `gosmi`. If users find issues with `gosmi` that do not occur
with `netsnmp` please open a project issue on GitHub.

The SNMP backend setting is a global-level setting that applies to all use of
SNMP in Telegraf. Users can set this option in the `[agent]` configuration via
the `snmp_translator` option. See the [agent configuration][AGENT] for more
details.

[AGENT]: ../../../docs/CONFIGURATION.md#agent

## Configuration

```toml @sample.conf
# Receive SNMP traps
[[instances]]
  ## Transport, local address, and port to listen on.  Transport must
  ## be "udp://".  Omit local address to listen on all interfaces.
  ##   example: "udp://127.0.0.1:1234"
  ##
  ## Special permissions may be required to listen on a port less than
  ## 1024.  See README.md for details
  ##
  # service_address = "udp://:162"
  ##
  ## Path to mib files
  ## Used by the gosmi translator.
  ## To add paths when translating with netsnmp, use the MIBDIRS environment variable
  # path = ["/usr/share/snmp/mibs"]
  ## default translator is netsnmp
  # translator = "gosmi"
  ## Deprecated in 1.20.0; no longer running snmptranslate
  ## Timeout running snmptranslate command
  # timeout = "5s"
  ## Snmp version
  # version = "2c"
  ## SNMPv3 authentication and encryption options.
  ##
  ## Security Name.
  # sec_name = "myuser"
  ## Authentication protocol; one of "MD5", "SHA" or "".
  # auth_protocol = "MD5"
  ## Authentication password.
  # auth_password = "pass"
  ## Security Level; one of "noAuthNoPriv", "authNoPriv", or "authPriv".
  # sec_level = "authNoPriv"
  ## Privacy protocol used for encrypted messages; one of "DES", "AES", "AES192", "AES192C", "AES256", "AES256C" or "".
  # priv_protocol = ""
  ## Privacy password used for encrypted messages.
  # priv_password = ""

  ## -----------------------------------------------------------------------
  ## Metric Aggregation and Mapping
  ## -----------------------------------------------------------------------

  ## Global Varbind Mapping (Optional)
  ## Replaces or renames varbinds matching the numeric OID prefix.
  # [instances.varbind_mapping]
  #   ".1.3.6.1.2.1.2.2.1.1" = "ifIndex"
  #   ".1.3.6.1.2.1.2.2.1.7" = "ifAdminStatus"

  ## Global Fields To Labels (Optional)
  ## Promotes translated varbinds matching these names into labels for the core metric
  ## and all dispersed metrics.
  # fields_to_labels = ["ifIndex", "ifAdminStatus", "ifOperStatus"]

  ## Trap Specific Mappings (Optional)
  ## Defines rules for specific traps. Takes precedence over global
  ## configurations for matched varbinds; unmatched varbinds still
  ## fall back to varbind_mapping and fields_to_labels.
  # [[instances.trap_mapping]]
  #   oid = ".1.3.6.1.6.3.1.1.5.3"
  #   name = "link_down"
  #   value = ".1.3.6.1.2.1.1.3"
  #
  #   [[instances.trap_mapping.varbind]]
  #     oid = ".1.3.6.1.2.1.2.2.1.1"
  #     name = "ifIndex"
```

### Using a Privileged Port

On many operating systems, listening on a privileged port (a port
number less than 1024) requires extra permission.  Since the default
SNMP trap port 162 is in this category, using telegraf to receive SNMP
traps may need extra permission.

Instructions for listening on a privileged port vary by operating
system. It is not recommended to run telegraf as superuser in order to
use a privileged port. Instead follow the principle of least privilege
and use a more specific operating system mechanism to allow telegraf to
use the port.  You may also be able to have telegraf use an
unprivileged port and then configure a firewall port forward rule from
the privileged port.

To use a privileged port on Linux, you can use setcap to enable the
CAP_NET_BIND_SERVICE capability on the telegraf binary:

```shell
setcap cap_net_bind_service=+ep /usr/bin/telegraf
```

On Mac OS, listening on privileged ports is unrestricted on versions
10.14 and later.

## Metrics

- snmp_trap
  - tags:
    - source (string, IP address of trap source)
    - name (string, value from SNMPv2-MIB::snmpTrapOID.0 PDU)
    - mib (string, MIB from SNMPv2-MIB::snmpTrapOID.0 PDU)
    - oid (string, OID string from SNMPv2-MIB::snmpTrapOID.0 PDU)
    - version (string, "1" or "2c" or "3")
    - context_name (string, value from v3 trap)
    - engine_id (string, value from v3 trap)

> **Migration Note**: The `community` tag (SNMP v1/v2c community string)
> has been removed from metric labels for security reasons. If your
> existing dashboards or alerts reference this label, please update them
> accordingly.
  - fields:
    - Fields are mapped from variables in the trap. Field names are
      the trap variable names after MIB lookup. Field values are trap
      variable values.

## Example Output

```text
snmp_trap,mib=SNMPv2-MIB,name=coldStart,oid=.1.3.6.1.6.3.1.1.5.1,source=192.168.122.102,version=2c snmpTrapEnterprise.0="linux",sysUpTimeInstance=1i 1574109187723429814
snmp_trap,mib=NET-SNMP-AGENT-MIB,name=nsNotifyShutdown,oid=.1.3.6.1.4.1.8072.4.0.2,source=192.168.122.102,version=2c sysUpTimeInstance=5803i,snmpTrapEnterprise.0="netSnmpNotificationPrefix" 1574109186555115459
```

## Metric Aggregation and Context Inheritance Tutorial

This section provides a concrete example to understand how to aggregate fragmented SNMP trap varbinds into unified, alert-friendly event metrics.

### 1. The SNMP Trap Simulation Command
We will use the following Linux `snmptrap` command to simulate a `linkDown` network event:

```bash
snmptrap -v 2c -c public 127.0.0.1:162 "" \
  .1.3.6.1.6.3.1.1.5.3 \
  .1.3.6.1.2.1.2.2.1.1.835 i 835 \
  .1.3.6.1.2.1.2.2.1.7.835 i 1 \
  .1.3.6.1.2.1.2.2.1.8.835 i 2
```

**Command breakdown:**
- `-v 2c -c public 127.0.0.1:162`: Use SNMP v2c, community "public", sending to localhost on port 162.
- `""`: The `sysUpTime` field. An empty string tells the CLI to automatically inject the current system uptime (e.g., `123456`).
- `.1.3.6.1.6.3.1.1.5.3`: The main Trap OID (`snmpTrapOID.0`), which translates to `linkDown` in SNMPv2-MIB.
- `.1.3.6.1.2.1.2.2.1.1.835 i 835`: Appended Varbind. OID translates to `ifIndex.835`. Type is `i` (Integer), Value is `835`.
- `.1.3.6.1.2.1.2.2.1.7.835 i 1`: Appended Varbind. Translates to `ifAdminStatus.835`, Value `1`.
- `.1.3.6.1.2.1.2.2.1.8.835 i 2`: Appended Varbind. Translates to `ifOperStatus.835`, Value `2`.

### 2. Configuration Case A: Explicit Core Value Mapping

In this configuration, we explicitly map the device's `sysUpTimeInstance` (`.1.3.6.1.2.1.1.3`) to act as the primary metric value.

```toml
[[instances]]
  service_address = "udp://0.0.0.0:162"
  fields_to_labels = ["ifIndex", "ifAdminStatus", "ifOperStatus"]

  [[instances.trap_mapping]]
    oid = ".1.3.6.1.6.3.1.1.5.3" 
    name = "link_down" 
    value = ".1.3.6.1.2.1.1.3"  # Maps sysUpTime as the core value
    
    [[instances.trap_mapping.varbind]]
      oid = ".1.3.6.1.2.1.2.2.1.1" 
      name = "ifIndex"
```

**Result:**
Because all varbinds sent in the trap were either absorbed as Labels or used as the Core Value, exactly **1 pure metric** is generated:
```text
snmp_trap_link_down{source="127.0.0.1",version="2c",oid=".1.3.6.1.6.3.1.1.5.3",name="link_down",mib="SNMPv2-MIB",ifIndex="835",ifAdminStatus="1",ifOperStatus="2"} = 123456
```

### 3. Configuration Case B: Context Inheritance (Dispersed Fallback)

In this configuration, we simply comment out the `value` specification, meaning the core metric value will default to `1`.

```toml
[[instances]]
  service_address = "udp://0.0.0.0:162"
  fields_to_labels = ["ifIndex", "ifAdminStatus", "ifOperStatus"]

  [[instances.trap_mapping]]
    oid = ".1.3.6.1.6.3.1.1.5.3" 
    name = "link_down" 
    # value = ".1.3.6.1.2.1.1.3"  <--- Commented Out
    
    [[instances.trap_mapping.varbind]]
      oid = ".1.3.6.1.2.1.2.2.1.1" 
      name = "ifIndex"
```

**Result:**
The `sysUpTimeInstance` varbind is no longer consumed by the `value` field, and it is not in the whitelist. According to the fallback logic, it drops into a dispersed metric. **Crucially, it seamlessly inherits all context labels extracted from the trap!**

Two metrics are generated:
```text
# 1. The Core Event Metric (perfect for '> 0' alerting)
snmp_trap_link_down{source="127.0.0.1",version="2c",oid=".1.3.6.1.6.3.1.1.5.3",name="link_down",mib="SNMPv2-MIB",ifIndex="835",ifAdminStatus="1",ifOperStatus="2"} = 1

# 2. The Dispersed Fallback Metric (inheriting the full context)
snmp_trap_sysUpTimeInstance{source="127.0.0.1",version="2c",oid=".1.3.6.1.6.3.1.1.5.3",name="link_down",mib="SNMPv2-MIB",ifIndex="835",ifAdminStatus="1",ifOperStatus="2"} = 123456
```

## References

- [net-snmp project home](http://www.net-snmp.org)
- [`snmpcmd` man-page](http://net-snmp.sourceforge.net/docs/man/snmpcmd.html)
