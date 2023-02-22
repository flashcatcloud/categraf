1. 需要提供
a. AccessKey
b. AcessSecret 
c. Endpoint 
d. RegionID
其中，Endpoint 与 RegionID 见[接入地址](https://help.aliyun.com/document_detail/28616.html?spm=a2c4g.11186623.0.0.30c85d7aFf1Qzc#section-72p-xhs-6qt)

请求限制:
 - Http Request Header+Query≤128KB
 - Http Request Body≤512KB
 - Http Response≤2048KB

2. 凭证相关
获取凭证 https://usercenter.console.aliyun.com/#/manage/ak
RAM 用户授权
RAM用户调用云监控API前，需要所属的阿里云账号将权限策略授予对应的RAM用户
ram用户权限见 https://help.aliyun.com/document_detail/43170.html?spm=a2c4g.11186623.0.0.30c841feqsoAAn
可以在[授权页面](https://ram.console.aliyun.com/permissions) 新增授权，选择对应的用户，
授予云监控只读权限AliyunCloudMonitorReadOnlyAccess, 授予权限的用户，创建accessKey 即可。


3. 指标查询
[阿里云监控指标](https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.3ad53c60q3sQz1)

4. 配置
```toml
# 阿里云资源所处的region
region="cn-beijing"
endpoint="metrics.cn-hangzhou.aliyuncs.com"
# 填入你的acces_key_id
access_key_id=""
# 填入你的access_key_secret
access_key_secret=""

# 可能无法获取当前最新指标，这个指标是指监控指标的截止时间距离现在多久
delay="50m"
# 采集周期，60s 是推荐值，再小了部分指标不支持
period="60s"
# 指标所属的namespace ,为空，则表示所有空间指标都要采集
namespaces=["acs_ecs_dashboard"]
# 过滤某个namespace下的一个或多个指标
[[instances.metric_filters]]
namespace=""
metric_names=["cpu_cores","vm.TcpCount", "cpu_idle"]

# 阿里云查询指标接口的QPS是50， 这里默认设置为一半
ratelimit=25
# 查询指定namesapce指标后, namespace/metric_name等meta信息会缓存起来，catch_ttl 是指标的缓存时间
catch_ttl="1h"
# 每次请求阿里云endpoint的超时时间
timeout="5s"

```