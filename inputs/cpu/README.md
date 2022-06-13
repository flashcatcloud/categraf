# cpu

CPU 采集插件很简单，自动采集本机 CPU 的使用率、空闲率等等，默认采集的是整机的，如果想采集单核的，就开启这个配置：

```ini
collect_per_cpu = true
```

其中 CPU 使用率的指标名字是 cpu_usage_active

## 监控大盘

该插件没有单独的监控大盘，OS 的监控大盘统一放到 system 下面了