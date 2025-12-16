# Huatuo Input Plugin

Categraf 的 Huatuo 插件主要提供以下两种功能：

1. **Sidecar 模式**: 以 Sidecar 模式管理本地 `huatuo-bamai` 进程的生命周期，包括安装、配置管理和进程保活。
2. **Remote 模式**: 仅采集远程或已存在的本地 `huatuo` 实例的监控指标。

## 配置说明

### Sidecar 模式

在此模式下，Categraf 将执行以下操作：
1. 检查 `install_path` 目录下是否存在 `huatuo-bamai` 二进制文件。
2. 如果缺失，且配置了 `huatuo_tarball`，则自动解压安装。
3. 读取安装目录下的 `huatuo-bamai.conf` (TOML 格式)。
4. 应用 `config_overwrites` 中的配置覆盖现有配置并保存。
5. 启动 `huatuo-bamai` 进程并进行后台保活。
6. 自动从 `huatuo-bamai.conf` 中解析指标监听端口 (字段 `APIServer.TCPAddr`) 并开始采集。

```toml
[[instances]]
# huatuo 安装或查找的目录路径
install_path = "./huatuo" 

# (可选) 当二进制缺失时用于自动安装的压缩包路径。
# 如果使用特定的 Categraf 发行版，这里通常是 "embedded/huatuo.tar.gz"。
huatuo_tarball = "embedded/huatuo.tar.gz"

# 覆盖 huatuo-bamai.conf 中的特定配置
[instances.config_overwrites]
"Storage.ES.Address" = "http://127.0.0.1:9200"
"Region" = "beijing"
"EventTracing.Softirq.DisabledThreshold" = 20000000
```

### Remote 模式

在此模式下，Categraf 仅负责采集指标，不管理进程。

```toml
[[instances]]
# 远程采集地址
url = "http://192.168.1.100:19704/metrics"

# install_path 必须为空或省略
# install_path = ""
```
