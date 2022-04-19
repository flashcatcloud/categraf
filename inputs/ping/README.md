### File Limit

```sh
systemctl edit categraf
```

Increase the number of open files:

```ini
[Service]
LimitNOFILE=8192
```

Restart Categraf:

```sh
systemctl restart categraf
```

### Linux Permissions

On most systems, ping requires `CAP_NET_RAW` capabilities or for Categraf to be run as root.

With systemd:

```sh
systemctl edit categraf
```

```ini
[Service]
CapabilityBoundingSet=CAP_NET_RAW
AmbientCapabilities=CAP_NET_RAW
```

```sh
systemctl restart categraf
```

Without systemd:

```sh
setcap cap_net_raw=eip /usr/bin/categraf
```

Reference [`man 7 capabilities`][man 7 capabilities] for more information about
setting capabilities.

[man 7 capabilities]: http://man7.org/linux/man-pages/man7/capabilities.7.html

### Other OS Permissions

When using `method = "native"`, you will need permissions similar to the executable ping program for your OS.



