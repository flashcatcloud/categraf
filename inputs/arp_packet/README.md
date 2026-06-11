# ARP Packet

This plugin captures ARP request and response packets on a specified network interface using a BPF filter, keeping track of the total packet counts for the local IP address.

> **Note**: Running this plugin requires packet capture capabilities (e.g., running as root or having `CAP_NET_RAW` capability) and libpcap dependencies.

## Configuration

```toml
# Collection interval in seconds
interval = 15

[[instances]]
# The name of the network interface to monitor
eth_device = "eth0"
```

### Finding the Interface Name

You can use the following command to get a list of available network interfaces:

```sh
ip addr | grep '^[0-9]' | awk -F':' '{print $2}'
```
Example output:
```text
 lo
 eth0
 docker0
```

Select the appropriate interface (e.g., `eth0`) and set it in the `eth_device` parameter.

## Metrics

- `arp_packet_request_num`: Total number of ARP requests sent from the monitored interface.
- `arp_packet_response_num`: Total number of ARP responses received on the monitored interface.

All metrics include the `sourceAddr` tag, which contains the bound local IPv4 address.

## Testing

You can use the following command to test if the plugin is successfully capturing ARP packets:

```sh
./categraf --test --inputs arp_packet
```
