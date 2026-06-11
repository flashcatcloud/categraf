# DNS Query Input Plugin

The DNS Query input plugin is used to continuously monitor the response quality of DNS servers. It helps operators quickly locate network latency and resolution errors caused by DNS queries.

**Deployment Recommendation:**
It is not necessary to enable this plugin on every machine. We recommend enabling it on core gateway nodes, specific network probe VMs, or central monitoring nodes to regularly query critical domain names.

## Configuration

```toml
[[instances]]
  # Automatically use the DNS servers from the local machine's /etc/resolv.conf
  auto_detect_local_dns_server = true

  ## Manually specify external DNS servers to query
  servers = ["223.5.5.5", "114.114.114.114", "119.29.29.29"]

  ## Network protocol to use, such as "udp" or "tcp"
  # network = "udp"

  ## List of domains or subdomains to query
  domains = ["www.huaweicloud.com", "www.baidu.com", "api.yourcompany.com"]

  ## Query record type (A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV)
  record_type = "A"

  ## DNS server port
  # port = 53

  ## Query timeout in seconds
  timeout = 5
```

If you need to query different record types (e.g., `A` records and `CNAME` records), you can configure multiple `[[instances]]` blocks.

## Metrics

- `dns_query_query_time_ms`: The latency/response time of the DNS resolution in milliseconds.
- `dns_query_result_code`: The result code of the probe execution (0 means success, non-zero indicates an exception like timeout or connection failure).
- `dns_query_rcode_value`: The standard DNS protocol response code (e.g., NOERROR, NXDOMAIN, SERVFAIL).

All metrics include tags such as `server`, `domain`, and `record_type`, allowing for granular analysis per DNS server or domain.

## Alerting Recommendations

- **P2 Alert**: Trigger when `dns_query_query_time_ms > 2000` ms.
- **P1 Alert**: Trigger when `dns_query_query_time_ms > 5000` ms.
- **Critical Alert**: Trigger when `dns_query_result_code != 0`, indicating DNS resolution failure.
