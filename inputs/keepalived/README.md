# Keepalived

forked from [keepalived-exporter](https://github.com/mehdy/keepalived-exporter)

## Configuration

请参考配置[示例](../../conf/input.keepalived/keepalived.toml)

```
# # collect interval
# interval = 15

# Set to true to enable this plugin (change false -> true).
enable = false

# Send SIGJSON and decode JSON file instead of parsing text files, defaults to `false`.
sig_json = false

# A path for Keepalived PID, defaults to `/var/run/keepalived.pid`
pid_path = ""

# Health Check script path to be execute for each VIP.
# Check Script Example:
#!/usr/bin/env bash
#ping $1 -c 1 -W 1
check_script_path = ""

#This is when the keepalived is running with PID 1 in the container so we can use the standard docker API to send signal to the keepalived process.
# Keepalived container name to export metrics from Keepalived container.
container_name = ""

# In case the keepalived process is not running with PID 1, this method will exec to the container and use the provided PID path to send the signal.
container_pid_path = ""

# Keepalived container tmp volume path, defaults to `/tmp`.
container_tmp = ""
```

#### Using Docker Signal

This is when the keepalived is running with PID 1 in the container so we can use the standard docker API to send signal to the keepalived process.

```bash
docker run -v keepalived-data:/tmp/ ... $KEEPALIVED_IMAGE
```

#### Exec to container with PID path

In case the keepalived process is not running with PID 1, this method will exec to the container and use the provided PID path to send the signal.

```bash
docker run -v keepalived-data:/tmp/ -v keepalived-pid:/var/run/ ... $KEEPALIVED_IMAGE
```

## Metrics

| Metric                                          | Notes
|-------------------------------------------------|------------------------------------
| keepalived_up                                   | Status of Keepalived service
| keepalived_vrrp_state                           | State of vrrp
| keepalived_vrrp_excluded_state                  | State of vrrp with excluded VIP
| keepalived_check_script_status                  | Check Script status for each VIP
| keepalived_gratuitous_arp_delay_total           | Gratuitous ARP delay
| keepalived_advertisements_received_total        | Advertisements received
| keepalived_advertisements_sent_total            | Advertisements sent
| keepalived_become_master_total                  | Became master
| keepalived_release_master_total                 | Released master
| keepalived_packet_length_errors_total           | Packet length errors
| keepalived_advertisements_interval_errors_total | Advertisement interval errors
| keepalived_ip_ttl_errors_total                  | TTL errors
| keepalived_invalid_type_received_total          | Invalid type errors
| keepalived_address_list_errors_total            | Address list errors
| keepalived_authentication_invalid_total         | Authentication invalid
| keepalived_authentication_mismatch_total        | Authentication mismatch
| keepalived_authentication_failure_total         | Authentication failure
| keepalived_priority_zero_received_total         | Priority zero received
| keepalived_priority_zero_sent_total             | Priority zero sent
| keepalived_script_status                        | Tracker Script Status
| keepalived_script_state                         | Tracker Script State

## Check Script

You can specify a check script like Keepalived script check to check if all the things is okay or not.
The script will run for each VIP and gives an arg `$1` that contains VIP.

**Note:** The script should be executable.

```bash
chmod +x check_script.sh
```

### Sample Check Script

```bash
#!/usr/bin/env bash
ping $1 -c 1 -W 1
```

## Example Output

```text
keepalived_up agent_hostname=zy-wh-fat-utils-test-06 1
keepalived_advertisements_received_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_advertisements_sent_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 8869
keepalived_become_master_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 1
keepalived_release_master_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_packet_length_errors_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_advertisements_interval_errors_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_ip_ttl_errors_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_invalid_type_received_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_address_list_errors_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_authentication_invalid_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_authentication_mismatch_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_authentication_failure_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_priority_zero_received_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_priority_zero_sent_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 0
keepalived_gratuitous_arp_delay_total agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 vrid=51 5
keepalived_vrrp_state agent_hostname=zy-wh-fat-utils-test-06 iname=VI_1 intf=ens3 ip_address=172.20.84.253/24 vrid=51 2
keepalived_script_status agent_hostname=zy-wh-fat-utils-test-06 name=chk_nginx 1
keepalived_script_state agent_hostname=zy-wh-fat-utils-test-06 name=chk_nginx 0
keepalived_scrape_use_seconds agent_hostname=zy-wh-fat-utils-test-06 0.000381182
```