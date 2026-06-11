# IPVS Input Plugin

Forked from Telegraf. The IPVS input plugin uses the Linux kernel netlink socket interface to gather metrics about IPVS virtual and real servers.

**Supported Platforms:** Linux

## Permissions

In order for this plugin to communicate over netlink sockets, the Categraf process needs to be running as `root` (or as a user with `CAP_NET_ADMIN` and `CAP_NET_RAW` capabilities). Be sure to ensure these permissions before running Categraf with this plugin included.

## Configuration

```toml
# Collect virtual and real server stats from Linux IPVS
[[instances]]
# No specific configuration is required.
```

## Metrics

Servers will contain tags identifying how they were configured, using either `address` + `port` + `protocol` *OR* `fwmark`. This corresponds to how you would normally configure a virtual server using `ipvsadm`.

### 1. ipvs_virtual_server
- **Tags:**
    - `sched` (the scheduler in use)
    - `netmask` (the mask used for determining affinity)
    - `address_family` (inet/inet6)
    - `address`
    - `port`
    - `protocol`
    - `fwmark`
- **Fields:**
    - `connections`
    - `pkts_in` / `pkts_out`
    - `bytes_in` / `bytes_out`
    - `pps_in` / `pps_out`
    - `cps`

### 2. ipvs_real_server
- **Tags:**
    - `address`
    - `port`
    - `address_family` (inet/inet6)
    - `virtual_address`
    - `virtual_port`
    - `virtual_protocol`
    - `virtual_fwmark`
- **Fields:**
    - `active_connections`
    - `inactive_connections`
    - `connections`
    - `pkts_in` / `pkts_out`
    - `bytes_in` / `bytes_out`
    - `pps_in` / `pps_out`
    - `cps`
