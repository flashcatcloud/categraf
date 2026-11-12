forked from [telegraf/gnmi](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/gnmi)

# gNMI (gRPC Network Management Interface) Plugin

This plugin consumes telemetry data based on the [gNMI][1] Subscribe method. TLS
is supported for authentication and encryption.  This plugin is
vendor-agnostic and is supported on any platform that supports the gNMI spec.

For Cisco devices:

It has been optimized to support gNMI telemetry as produced by Cisco IOS XR
(64-bit) version 6.5.1, Cisco NX-OS 9.3 and Cisco IOS XE 16.12 and later.

[1]: https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md

Please check the [troubleshooting section](#troubleshooting) in case of
problems, e.g. when getting an *empty metric-name warning*!

## Configuration

```toml @gnmi.toml
# gNMI telemetry plugin
[[instances]]
  ## Address and port of the gNMI GRPC server
  addresses = ["1.2.3.4:5678"]

  ## define credentials
  username = "admin"
  password = "admin"

  ## gNMI encoding requested (one of: "proto", "json", "json_ietf", "bytes")
    encoding = "proto"

  ## redial in case of failures after
    redial = "10s"

  ## gRPC Maximum Message Size, default 4MB 
    max_msg_size = "4194304"

  ## Enable to get the canonical path as field-name
  # canonical_field_names = false

  ## Remove leading slashes and dots in field-name
  # trim_field_names = false

  ## Guess the path-tag if an update does not contain a prefix-path
  ## If enabled, the common-path of all elements in the update is used.
  # guess_path_tag = false

  ## enable client-side TLS and define CA to authenticate the device
  # enable_tls = false
  # tls_ca = "/etc/pki/ca.pem"
  ## Minimal TLS version to accept by the client
  # tls_min_version = "TLS12"
  ## Use TLS but skip chain & host verification
  # insecure_skip_verify = true

  ## define client-side TLS certificate & key to authenticate to the device
  # tls_cert = "/etc/pki/cert.pem"
  # tls_key = "/etc/pki/key.pem"

  ## gNMI subscription prefix (optional, can usually be left empty)
  ## See: https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md#222-paths
  # origin = ""
  # prefix = ""
  # target = ""

  ## Vendor specific options
  ## This defines what vendor specific options to load.
  ## * Juniper Header Extension (juniper_header): some sensors are directly managed by
  ##   Linecard, which adds the Juniper GNMI Header Extension. Enabling this
  ##   allows the decoding of the Extension header if present. Currently this knob
  ##   adds component, component_id & sub_component_id as additionnal tags
  # vendor_specific = []

  ## Define additional aliases to map encoding paths to measurement names
  # [instances.aliases]
  #   ifcounters = "openconfig:/interfaces/interface/state/counters"

  [[instances.subscription]]
    ## Name of the measurement that will be emitted
    name = "ifcounters"

    ## Origin and path of the subscription
    ## See: https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-specification.md#222-paths
    ##
    ## origin usually refers to a (YANG) data model implemented by the device
    ## and path to a specific substructure inside it that should be subscribed
    ## to (similar to an XPath). YANG models can be found e.g. here:
    ## https://github.com/YangModels/yang/tree/master/vendor/cisco/xr
    origin = "openconfig-interfaces"
    path = "/interfaces/interface/state/counters"

    ## Subscription mode ("target_defined", "sample", "on_change") and interval
    subscription_mode = "sample"
    sample_interval = "10s"

    ## Suppress redundant transmissions when measured values are unchanged
    # suppress_redundant = false

    ## If suppression is enabled, send updates at least every X seconds anyway
    # heartbeat_interval = "60s"

  ## Tag subscriptions are applied as tags to other subscriptions.
  # [[instances.tag_subscription]]
  #  ## When applying this value as a tag to other metrics, use this tag name
  #  name = "descr"
  #
  #  ## All other subscription fields are as normal
  #  origin = "openconfig-interfaces"
  #  path = "/interfaces/interface/state"
  #  subscription_mode = "on_change"
  #
  #  ## Match strategy to use for the tag.
  #  ## Tags are only applied for metrics of the same address. The following
  #  ## settings are valid:
  #  ##   unconditional -- always match
  #  ##   name          -- match by the "name" key
  #  ##                    This resembles the previsou 'tag-only' behavior.
  #  ##   elements      -- match by the keys in the path filtered by the path
  #  ##                    parts specified `elements` below
  #  ## By default, 'elements' is used if the 'elements' option is provided,
  #  ## otherwise match by 'name'.
  #  # match = ""
  #
  #  ## For the 'elements' match strategy, at least one path-element name must
  #  ## be supplied containing at least one key to match on. Multiple path
  #  ## elements can be specified in any order. All given keys must be equal
  #  ## for a match.
  #  # elements = ["description", "interface"]
  #
  #  ## Optional: remove the source leaf's identically named key on its
  #  ## immediate parent list after a successful tag replacement. Default: false.
  #  ## Example: /interface/name with name="ifname" replaces the interface key.
  #  ## Requires matching ancestry/origin and equal values. Missing/conflicting
  #  ## tags preserve the original key; other list keys are unaffected.
  #  # drop_source_key = false
```

## Metrics

Each configured subscription will emit a different measurement.  Each leaf in a
GNMI SubscribeResponse Update message will produce a field reading in the
measurement. GNMI PathElement keys for leaves will attach tags to the field(s).

## Example Output

```text
ifcounters,path=openconfig-interfaces:/interfaces/interface/state/counters,host=linux,name=MgmtEth0/RP0/CPU0/0,source=10.49.234.115,descr/description=Foo in-multicast-pkts=0i,out-multicast-pkts=0i,out-errors=0i,out-discards=0i,in-broadcast-pkts=0i,out-broadcast-pkts=0i,in-discards=0i,in-unknown-protos=0i,in-errors=0i,out-unicast-pkts=0i,in-octets=0i,out-octets=0i,last-clear="2019-05-22T16:53:21Z",in-unicast-pkts=0i 1559145777425000000
ifcounters,path=openconfig-interfaces:/interfaces/interface/state/counters,host=linux,name=GigabitEthernet0/0/0/0,source=10.49.234.115,descr/description=Bar out-multicast-pkts=0i,out-broadcast-pkts=0i,in-errors=0i,out-errors=0i,in-discards=0i,out-octets=0i,in-unknown-protos=0i,in-unicast-pkts=0i,in-octets=0i,in-multicast-pkts=0i,in-broadcast-pkts=0i,last-clear="2019-05-22T16:54:50Z",out-unicast-pkts=0i,out-discards=0i 1559145777425000000
```

## Troubleshooting

Some devices (e.g. Juniper) report spurious data with response paths not
corresponding to any subscription. In those cases, Categraf will not be able
to determine the metric name for the response and you get an
*empty metric-name warning*

For examplem if you subscribe to `/junos/system/linecard/cpu/memory` but the
corresponding response arrives with path
`/components/component/properties/property/...` To avoid those issues, you can
manually map the response to a metric name using the `aliases` option like

```toml
[[insances]]
  addresses     = ["..."]

  [instances.aliases]
    memory = "/components"

  [[instances.subscription]]
    name = "memory"
    origin = "openconfig"
    path = "/junos/system/linecard/cpu/memory"
    subscription_mode = "sample"
    sample_interval = "60s"
```

If this does *not* solve the issue, please follow the warning instructions and
open an issue with the response, your configuration and the metric you expect.

## Built-in session health

The local gNMI plugin reports one `gnmi_up` series per configured `address`
(host:port), across files and instances. The value is 1 when any initialized
subscription for that address has received a valid notification or sync response
and its session remains active; otherwise it is 0. This is endpoint health, so
one healthy session can coexist with failures in other subscriptions.

Set `disable_gnmi_up` and `gnmi_up_interval` once at the plugin root alongside
`interval`, before the first `[[instances]]`. For split TOML files, put these
settings in the first file (for example, `00-gnmi.toml`), since local TOML files
are concatenated in filename order. Move existing instance-level health options
to the plugin root; the old values no longer control reporting.

The first plugin Gather emits a sample. A state change emits on the next
Gather and restarts the cadence; an unchanged state emits after
`gnmi_up_interval` (default 30s, including when set to zero). Negative intervals
are invalid. Reporting is limited by the plugin Gather cadence. Both changes in
a short outage between Gather calls may go unobserved.

Health uses the endpoint address and plugin/global label processing, without
instance labels or instance relabel rules. If a plugin label owns `address`,
endpoint identity uses a free `gnmi_target`, `gnmi_target_1`, etc. instead.
Configured labels are preserved, accounting for normalized plugin/global names.
Keep the resulting identity label distinct in the final label set. Separate
HTTP-delivered plugin objects and agent processes report independently. Business
sampling remains at instance level.

## Optional built-in ICMP

ICMP is disabled by default. Set `enable_icmp = true` once at the plugin root,
alongside `interval`, before any instance tables. Move any previous instance
settings to the plugin root. Legacy instance options are accepted but ignored
and non-default values produce a migration warning.

The monitor probes each distinct host from initialized instances, across local
files and ports. It skips targets without an extractable host:port. It uses the
configured hostname or IP as the identity, without resolving aliases for deduplication.

Defaults are `icmp_interval = "30s"`, `icmp_timeout = "3s"`, `icmp_count = 3`,
`icmp_packet_interval = "200ms"`, `icmp_concurrency = 10`, `icmp_privileged = true`,
and `icmp_report_interval = "60s"`. The timeout includes DNS. Privileged probes
require appropriate socket permissions. The monitor continues probing independently
of Gather. The first completed result and changes between reachable, unreachable,
and probe failure are sent on the next Gather. Stable results send only the latest
new round once the report interval has elapsed. An expired result emits one failure
sample group, without repeating stale measurements.

ICMP metrics normally use `icmp_host` instead of a gNMI endpoint address. If a
plugin label owns that name, or a global alias such as `icmp-host` would normalize
to it, host identity uses a free `gnmi_icmp_host`, `gnmi_icmp_host_1`, etc.
Existing labels remain intact. An exact global `icmp_host` default does not
override the emitted identity, matching normal global-label precedence.
A completed round emits `gnmi_icmp_probe_success` and `gnmi_icmp_last_probe_timestamp_seconds`.
A successful round adds `gnmi_icmp_up` and `gnmi_icmp_packet_loss_ratio`.
All packets lost means probe success 1, up 0 and loss ratio 1. If a reply is
received, `gnmi_icmp_rtt_seconds` is the round's average RTT. Execution errors
and expired results omit reachability, loss and RTT. Update dashboard selectors
from `address` to `icmp_host` when migrating ICMP queries, or to the fallback
identity label if your configured labels require one.

## Complete configuration examples

Adjust the address, credentials, TLS, origin and paths for your device. Put all
instance options before subscription or tag-subscription subtables.

### Example 1: session up only, 30-second stable-state reporting

```toml
interval = "1s"
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = false
[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
# username = "your-user"
# password = "your-password"
[[instances.subscription]]
name = "ifcounters"
origin = "openconfig-interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"
```

Only gnmi_up is emitted for session health. The first Gather and observed state
changes emit immediately; unchanged states are emitted after 30 seconds.
Telemetry sampling and Gather forwarding keep their independent schedules.

### Example 2: session up with optional ICMP

```toml
interval = "1s"
disable_gnmi_up = false
gnmi_up_interval = "30s"
enable_icmp = true
icmp_interval = "30s"
icmp_timeout = "3s"
icmp_count = 3
icmp_report_interval = "60s"
[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
[[instances.subscription]]
name = "ifcounters"
origin = "openconfig-interfaces"
path = "/interfaces/interface/state/counters"
subscription_mode = "sample"
sample_interval = "15s"
```

ICMP probes run independently of telemetry. Only new results are eligible for reporting.

### Example 3: replace interface and queue source keys with learned tags

This standalone configuration requires device-supported RDMA paths. Adjust the
address, credentials, TLS, origin and paths for your model.

```toml
interval = "1s"

[[instances]]
addresses = ["192.0.2.10:50051"]
agent_host_tag = "ident"
# username = "your-user"
# password = "your-password"

[[instances.subscription]]
name = "rdma_input"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/input/queues/queue/state"
subscription_mode = "sample"
sample_interval = "15s"

[[instances.tag_subscription]]
name = "ifname"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/name"
subscription_mode = "sample"
sample_interval = "15s"
match = "elements"
elements = ["interface"]
disable_concatenation = true
drop_source_key = true

[[instances.tag_subscription]]
name = "qename"
origin = "openconfig-qos"
path = "/qos/rdma-counters/interfaces/interface/input/queues/queue/name"
subscription_mode = "sample"
sample_interval = "15s"
match = "elements"
elements = ["interface", "queue"]
disable_concatenation = true
drop_source_key = true
```

After a successful equal-valued tag lookup, ifname replaces the interface name
key and qename replaces the input queue's full-path name key. Other keys such as
id remain. Missing/conflicting tags, mismatched sources or unequal values retain
the original key. Enable drop_source_key separately in each applicable tag
subscription; it defaults to false. For output queues, add matching telemetry and
tag subscriptions with input replaced by output. If using session health/ICMP,
put those instance options before all subscription subtables.
