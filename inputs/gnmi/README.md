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

Session health is emitted every Gather even when no telemetry arrives. Set
`disable_gnmi_up = true` to disable all three session metrics:

- `gnmi_up`: 1 after a valid Update notification or `sync_response=true` in the current session; 0 before confirmation, during reconnect, or after EOF/RPC/protocol error.
- `gnmi_last_update_timestamp_seconds`: local receive time of the last notification containing Update/Delete data (including tag updates), or 0.
- `gnmi_reconnects_total`: actual connection attempts after the first; resets when the instance restarts.

Times survive reconnects. `gnmi_up` does not guarantee fresh or successfully
converted business metrics. Normal ON_CHANGE silence leaves it at 1; an
undetected blackholed connection can also remain 1. No extra RPCs or automatic
silence timeout are introduced.

Health labels contain static instance labels, the configured `agent_host_tag`
(default `agent_host`), and `address` (full host:port). Static labels must not
use either reserved identity key, and `agent_host_tag` must not be `address`.
Addresses must be unique host:port endpoints; IPv6 requires brackets. Use distinct
static labels for the same address in different instances. Normal pipeline
filters, renaming, global labels and relabel rules still apply.

## Optional built-in ICMP

Set `enable_icmp = true` before subscription subtables. Defaults:
`icmp_interval="30s"`, `icmp_timeout="3s"` (entire round including DNS),
`icmp_count=3`, `icmp_packet_interval="200ms"`, `icmp_concurrency=10`,
`icmp_privileged=true`. Omitted/zero options use defaults; negative values,
interval <= timeout, or timeout <= (count-1)*packet interval are invalid.
Disabled ICMP does not validate its unused options or perform DNS/network work.
Raw ICMP needs OS permissions (for example CAP_NET_RAW on Linux or appropriate
Windows privileges); `icmp_privileged=false` requests unprivileged ICMP sockets
where supported. Runtime probe errors never stop gNMI collection.

| Metric | Meaning |
|---|---|
| gnmi_icmp_probe_success | 1 for a completed valid round, 0 if pending, execution failed or result expired |
| gnmi_icmp_up | 1 if at least one reply, otherwise 0; only emitted for valid current results |
| gnmi_icmp_rtt_seconds | Mean successful reply RTT in seconds; omitted if no replies; 0 is a valid clock-resolution result |
| gnmi_icmp_packet_loss_ratio | Lost/sent, 0..1; all lost = 1; only valid current results |
| gnmi_icmp_last_probe_timestamp_seconds | Last completed round (including execution failures), local Unix seconds; 0 before first completion |

All-packet timeout after sending the requested count is a successful probe
execution with up=0/loss=1. Permission, DNS and socket errors or incomplete sends
have probe_success=0 and omit up/RTT/loss. Failed rounds replace old success.
Results older than 2*interval+timeout are treated as stale and omit up/RTT/loss.
Repeated Gather snapshots carry the original probe completion time separately.

ICMP uses a bounded background worker pool, never blocks Gather, and never
changes subscription state or triggers reconnects. Same-instance endpoints with
the same normalized hostname share probes but retain separate address labels.
The first probe is spread over [0, interval); subsequent probes wait interval
with +/-10% jitter after completion; no host has overlapping rounds. DNS is
resolved each round; IPv4 is preferred, then IPv6, with stable sorting within a
family. Only that selected address is tested; DNS aliases and separate instances
do not share probes. IPv6 literals with zone are supported. Health identity
validation applies when either session metrics or ICMP is enabled; disabling
both retains legacy endpoint/static-label handling.

For optional real loopback validation, run with `GNMI_TEST_ICMP=1`:
`go test ./inputs/gnmi -run TestICMPNativeLoopback -v -count=1`.
Normal tests need no ICMP privileges. Validate device ACLs, packet loss, IPv6,
and runtime permissions in the deployment environment before enabling alarms.

Implementation note: the ICMP backend uses the repository's existing
`golang.org/x/net/icmp` directly. The locked pro-bing version cannot reliably
bound all send retries, so it is not used by gNMI. Every write has a deadline;
any send error invalidates the round. `icmp_count` must not exceed 65535.
