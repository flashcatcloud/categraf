#!/bin/bash

# 脚本用途：检测nginx配置是否异常
# 告警条件：conf_status_code=1为异常

# 监控指标名
input_name="nginx"

# 自定义标签
cloud="my-cloud"
region="my-region"
azone="az1"
product="my-product"

nginx_service=$(/usr/sbin/nginx -t > /dev/null 2>&1)
if [ $? -eq 0 ];then
        conf_status_code=0
else
        conf_status_code=1
fi

echo "${input_name},cloud=${cloud},region=${region},azone=${azone},product=${product} conf_status_code=${conf_status_code}"
