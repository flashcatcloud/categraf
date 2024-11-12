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