#!/bin/bash

# 脚本用途：检测虚拟机登录用户数是否异常

# 监控指标名
input_name="system"

# 自定义标签
cloud="my-cloud"
region="my-region"
azone="az1"
product="my-product"

ssh_conn_count=`who | wc -l`
echo "${input_name},cloud=${cloud},region=${region},azone=${azone},product=${product} ssh_conn_count=${ssh_conn_count}"
