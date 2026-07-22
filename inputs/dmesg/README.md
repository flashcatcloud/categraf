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

  # How many retained kernel messages to read on startup.
  # "0s" reads only new messages after Categraf starts.
  # "1h" reads retained messages from the last hour, then new messages.
  # "-1" reads all retained messages in the current ring buffer.
  startup_lookback = "0s"

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
plugin instance restarts.

By default, `startup_lookback = "0s"` seeks to the end of `/dev/kmsg` during
startup and counts only messages generated after Categraf starts. This avoids
counting old kernel errors again when Categraf restarts on a long-running host.

If `startup_lookback` is set to a positive duration such as `"1h"`, the plugin
reads the current kernel ring buffer from the beginning but only counts messages
whose `/dev/kmsg` monotonic timestamp is within that lookback window. This is a
relative duration filter and does not use wall-clock time or timezone
conversion. A large lookback window can still count recent messages again after
a Categraf restart.

Set `startup_lookback = "-1"` only when you explicitly want to read all retained
messages in the current kernel ring buffer.

For alerting, prefer a range expression such as `increase(dmesg_hit_keyword[5m])
> 0` to detect newly counted messages. A direct `dmesg_hit_keyword > 0` check is
sticky for the life of the Categraf process once a matching message has been
counted.

## Testing

Run Categraf with sufficient permission to read `/dev/kmsg`:

```sh
./categraf --test --inputs dmesg
```
