# Filecount 采集插件

该插件用于统计指定目录下符合条件的文件的数量和总大小。该插件 fork 自 `telegraf/inputs.filecount`。

## 配置说明

```toml
[[instances]]
# # 待统计的目标目录
# 支持标准 glob 匹配规则，并增加 ** 作为超级通配符：
#   /var/log/**    -> 递归查找 /var/log 下的所有目录，并统计每个目录内的文件
#   /var/log/*/*   -> 查找父目录为 /var/log 的所有目录，并统计每个目录内的文件
#   /var/log       -> 统计 /var/log 及其所有子目录中的所有文件总和
directories = ["/tmp", "/root"]

# # 文件名匹配模式。默认为 "*"。
file_name = "*"

# # 是否统计子目录中的文件。默认为 true。
recursive = true

# # 是否仅统计普通文件 (排除目录、符号链接、Socket等)。默认为 true。
regular_only = true

# # 遍历目录树时是否跟随符号链接。默认为 false。
follow_symlinks = false

# # 按文件大小过滤。
# 只有大于等于此大小的文件才会被统计。
# 如果配置为负数，则只统计小于其绝对值的文件。
# 支持的单位有 B, KiB, MiB, KB 等...
# 如果不带引号和单位，则默认为字节。
size = "0B"

# # 按修改时间过滤。
# 只有在此时间之前（未被修改的时间超过该值）的文件才会被统计。
# 如果配置为负数，则只统计在此时长内被修改过的文件。默认为 "0s"。
mtime = "0s"
```

## 采集指标

所有指标将附带 `directory` 标签表示具体的目录路径。

- `filecount_count`: 匹配到的文件总数
- `filecount_size_bytes`: 匹配到的文件总大小 (Bytes)
- `filecount_oldest_file_timestamp`: 最早创建/修改的文件的 Unix 时间戳 (纳秒)
- `filecount_newest_file_timestamp`: 最新创建/修改的文件的 Unix 时间戳 (纳秒)
