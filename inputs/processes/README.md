# Processes Input Plugin

This plugin counts the total number of processes in the operating system categorized by their current state. For example, it tracks how many processes are Running, Sleeping, or Zombie.

**Supported Platforms:** Linux, FreeBSD, OpenBSD, macOS

*Note: This plugin is NOT supported on Windows.*

## Configuration

In most cases, no specific configuration is required; just leave it enabled.

```toml
# Collect OS process state distributions
# No specific configuration required
```

## Metrics

All metrics are prefixed with `processes_`. Key metrics include but are not limited to:

- `processes_total`: Total number of processes in the system
- `processes_running`: Number of running processes
- `processes_sleeping`: Number of sleeping processes
- `processes_zombies`: Number of zombie processes
- `processes_stopped`: Number of stopped processes
- `processes_paging`: Number of paging processes
- `processes_dead`: Number of dead processes
- `processes_idle`: Number of idle processes
- `processes_total_threads`: Same as above, total number of threads

## Dashboards

These metrics are part of basic host monitoring. Typically, process counts are grouped under a global **System** dashboard alongside CPU and memory.
A basic companion Dashboard focusing strictly on OS process states is also provided in this directory.
