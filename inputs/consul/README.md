# Consul Input Plugin

This plugin will collect statistics about all health checks registered in the
Consul. It uses [Consul API][1] to query the data. It will not report the
[telemetry][2] but Consul can report those stats already using StatsD protocol
if needed.

[1]: https://www.consul.io/docs/agent/http/health.html#health_state

[2]: https://www.consul.io/docs/agent/telemetry.html

## Global configuration options <!-- @/docs/includes/plugin_config.md -->

In addition to the plugin-specific configuration settings, plugins support
additional global and plugin configuration settings. These settings are used to
modify metrics, tags, and field or create aliases and configure ordering, etc.
See the [CONFIGURATION.md][CONFIGURATION.md] for more details.

[CONFIGURATION.md]: ../../../docs/CONFIGURATION.md#plugins

## Configuration

```toml @sample.conf
# Gather health check statuses from services registered in Consul
[[inputs.consul]]
  ## Consul server address
  # address = "localhost:8500"

  ## URI scheme for the Consul server, one of "http", "https"
  # scheme = "http"

  ## ACL token used in every request
  # token = ""

  ## HTTP Basic Authentication username and password.
  # username = ""
  # password = ""

  ## Data center to query the health checks from
  # datacenter = ""

  ## Optional TLS Config
  # tls_ca = "/etc/categraf/ca.pem"
  # tls_cert = "/etc/categraf/cert.pem"
  # tls_key = "/etc/categraf/key.pem"
  ## Use TLS but skip chain & host verification
  # insecure_skip_verify = true
```

## Example Output

```text
consul_up address=localhost:8500 agent_hostname=hostname 1
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=passing 1
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=warning 0
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=critical 0
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=maintenance 0
consul_health_checks_service_tag address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo tag=tag1 1
consul_health_checks_service_tag address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo tag=tag2 1
consul_scrape_use_seconds address=localhost:8500 agent_hostname=hostname 0.015674053
```
