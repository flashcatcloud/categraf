# Dmesg Input Plugin

This plugin reads Linux kernel messages from `/dev/kmsg` and counts messages
containing known error keywords or user-defined keywords. Keyword matching is a
case-sensitive substring match.

**Supported platform:** Linux only.

## Prerequisites

The Categraf process must be able to open and read `/dev/kmsg`. Running as root
usually provides the required access. If Categraf runs as an unprivileged user
or in a container, configure the service or container so that `/dev/kmsg` is
available and readable. Otherwise, initialization fails and Categraf writes an
`Error opening /dev/kmsg` message to its log.

## Configuration

The default configuration file is `conf/input.dmesg/dmesg.toml`. The plugin is
disabled by default because the file contains no active `[[instances]]` table.
With the default file, running `./categraf --debug --inputs dmesg` logs:

```text
W! no instances for input:dmesg
```

To enable the plugin, uncomment `[[instances]]` and configure it as needed:

```toml
# Collection interval in seconds. The global interval is used when omitted.
# interval = 15

[[instances]]
  # Instance interval = global/plugin interval * interval_times.
  # interval_times = 1

  # Additional case-sensitive substrings to count.
  # Do not configure an empty string because it matches every message.
  external_keywords = [
    "I/O error",
    "task blocked for more than",
  ]
```

The built-in keywords are always enabled:

- `Out of memory`
- `nf_conntrack: table full`
- `dropping packet`
- `will reset adapter`
- `memory error`
- `Reset successful for scsi`
- `Call Trace`
- `segfault`
- `NIC Link is Down`
- `EXT4-fs error`
- `Medium Error`
- `Package temperature above threshold`

## Metrics

All metrics are prefixed with `dmesg_`:

| Metric | Labels | Description |
| --- | --- | --- |
| `dmesg_up` | none | `1` when the current read succeeds; `0` when reading `/dev/kmsg` fails. An error opening `/dev/kmsg` occurs during initialization and is reported in the Categraf log. |
| `dmesg_hit_keyword` | `keyword` | Cumulative number of messages containing the keyword. One time series is emitted for every built-in and configured keyword, including keywords whose count is `0`. |

`dmesg_hit_keyword` is accumulated in memory and resets when Categraf or the
plugin instance restarts. On startup, the plugin consumes the kernel messages
that are available through the newly opened `/dev/kmsg` reader, then counts new
messages as they arrive.

## Testing

Run Categraf with sufficient permission to read `/dev/kmsg`:

```sh
./categraf --test --inputs dmesg
```
