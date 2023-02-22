1. 需要提供
AccessKey
AcessSecret 
Endpoint 
RegionID
其中，Endpoint 与 RegionID 见[接入地址](https://help.aliyun.com/document_detail/28616.html?spm=a2c4g.11186623.0.0.30c85d7aFf1Qzc#section-72p-xhs-6qt)

Http Request Header+Query≤128KB
Http Request Body≤512KB
Http Response≤2048KB

2. 凭证
获取凭证 https://usercenter.console.aliyun.com/#/manage/ak
RAM 用户授权
RAM用户调用云监控API前，需要所属的阿里云账号将权限策略授予对应的RAM用户
ram用户权限见 https://help.aliyun.com/document_detail/43170.html?spm=a2c4g.11186623.0.0.30c841feqsoAAn
可以在[授权页面](https://ram.console.aliyun.com/permissions) 新增授权，选择对应的用户，
授予云监控只读权限AliyunCloudMonitorReadOnlyAccess, 授予权限的用户，创建accessKey 即可。

`DescribeProjectMeta`
查询接入的云产品信息，包括产品的描述信息、Namespace和标签，单个API限速20次/秒， 主账号与所有RAM用户共享。
用户都默认接入

`DescribeMetricMetaList`
查询指定namespace和label下的指标监控项描述，单个API限速20次/秒， 主账号与所有RAM用户共享。

`DescribeMetricList`
查询指定云产品的指定监控项的监控数据。 单API限速50次/秒，主账号与所有ram用户共享。
DescribeMetricLast(获取指定监控项的最新监控数据),接口的最大限速是30
用户都默认开通接入

[阿里云监控指标](https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.3ad53c60q3sQz1)


`DescribeMonitoringAgentHosts`
查询所有已安装和未安装云监控插件的主机列表，可以获取云主机hostname与instance id映射关系等基础信息 ,单个API限速20次/秒， 主账号与所有RAM用户共享。
返回数据
{
"AliUid": 1082109605037616,
"HostName": "flashcat-saas-04",
"InstanceId": "i-2zeit7uza22wmlaljgph",
"InstanceTypeFamily": "ecs.c6e",
"IpGroup": "123.56.8.183,10.101.214.50",
"NetworkType": "vpc",
"OperatingSystem": "Linux",
"Region": "cn-beijing",
"SerialNumber": "32f55881-e5f6-4632-9eef-f95ba459fcaf",
"isAliyunHost": true
}

