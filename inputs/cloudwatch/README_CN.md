# Amazon CloudWatch 采集插件

该插件 fork 自 [telegraf/cloudwatch](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/cloudwatch)，用于从 Amazon CloudWatch 提取指标统计数据。

## 认证方式

此插件使用凭据链来与 CloudWatch API 进行认证。插件将按以下顺序尝试进行身份验证：

1. 如果指定了 `role_arn`，则通过 STS 承担角色（源凭证根据后续规则评估）
2. 如果配置了 `access_key`, `secret_key` 和 `token`，则使用显式凭据
3. 如果配置了 `profile`，则使用共享配置中的身份凭证
4. 环境变量（如 `AWS_ACCESS_KEY_ID` 等）
5. 共享凭据文件 (`~/.aws/credentials`)
6. EC2 实例配置文件 (IAM 角色)

## 配置说明

```toml
# 从 Amazon CloudWatch 提取指标统计数据
[[instances]]
  ## Amazon 区域
  region = "us-east-1"

  ## Amazon 凭据配置 (可选)
  # access_key = ""
  # secret_key = ""
  # token = ""
  # role_arn = ""
  # web_identity_token_file = ""
  # role_session_name = ""
  # profile = ""
  # shared_credential_file = ""

  ## 获取指标的周期 (必需)
  ## 必须是 60 秒的倍数。
  period = "5m"

  ## 采集延迟时间 (必需)
  ## 用于应对 CloudWatch API 中的指标生成延迟。
  delay = "5m"

  ## 建议: 将 interval 设置为 period 的倍数，以避免数据遗漏或重复抓取。
  interval = "5m"

  ## 指标所在的 Namespace 列表 (必需)
  namespaces = ["AWS/ELB"]

  ## 请求 CloudWatch API 的速率限制
  # ratelimit = 25

  ## CloudWatch HTTP 客户端超时时间
  # timeout = "5s"

  ## 指标配置过滤
  ## 默认拉取整个 Namespace 下的所有指标
  # [[instances.metrics]]
  #  names = ["Latency", "RequestCount"]
  #
  #  ## 指定指标获取的统计信息
  #  # statistic_include = ["average", "sum", "minimum", "maximum", "sample_count"]
  #
  #  ## Dimension (维度) 过滤条件
  #  [[instances.metrics.dimensions]]
  #    name = "LoadBalancerName"
  #    value = "p-example"
```

## 采集指标

监控的每个 CloudWatch Namespace 会作为 measurement，并提取相应的统计字段（命名为 `snake_case`）：

- `cloudwatch_{namespace}`
  - `{metric}_sum`         (总和)
  - `{metric}_average`     (平均值)
  - `{metric}_minimum`     (最小值)
  - `{metric}_maximum`     (最大值)
  - `{metric}_sample_count` (采样数)

### 标签 (Tags)

所有的指标都会被打上以下标签：
- `region`: CloudWatch 所在的区域
- `{dimension-name}`: 维度名称及对应的值
