# Filecount Input Plugin

forked from [telegraf/inputs.filecount](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/filecount)

Reports the number and total size of files in specified directories.


## Configuration

```toml filecount.toml
# # collect interval
# interval = 15

[[instances]]
# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Directories to gather stats about.
## This accept standard unit glob matching rules, but with the addition of
## ** as a "super asterisk". ie:
##   /var/log/**    -> recursively find all directories in /var/log and count files in each directories
##   /var/log/*/*   -> find all directories with a parent dir in /var/log and count files in each directories
##   /var/log       -> count all files in /var/log and all of its subdirectories
## directories = ["/var/cache/apt", "/tmp"]
directories = ["/tmp", "/root"]

## Only count files that match the name pattern. Defaults to "*".
file_name = "*"

## Count files in subdirectories. Defaults to true.
recursive = true

## Only count regular files. Defaults to true.
regular_only = true

## Follow all symlinks while walking the directory tree. Defaults to false.
follow_symlinks = false

## Only count files that are at least this size. If size is
## a negative number, only count files that are smaller than the
## absolute value of size. Acceptable units are B, KiB, MiB, KB, ...
## Without quotes and units, interpreted as size in bytes.
size = "0B"

## Only count files that have not been touched for at least this
## duration. If mtime is negative, only count files that have been
## touched in this duration. Defaults to "0s".
mtime = "0s"

```

## Metrics

- filecount
  - tags:
    - directory (the directory path)
  - fields:
    - count (integer)
    - size_bytes (integer)
    - oldest_file_timestamp (int, unix time nanoseconds)
    - newest_file_timestamp (int, unix time nanoseconds)

## Example Output

```text
13:25:07 filecount_count agent_hostname=host1 directory=/tmp 319
13:25:07 filecount_size_bytes agent_hostname=host1 directory=/tmp 83196547
13:25:07 filecount_oldest_file_timestamp agent_hostname=host1 directory=/tmp 0
13:25:07 filecount_newest_file_timestamp agent_hostname=host1 directory=/tmp 1692336254306413522
```
