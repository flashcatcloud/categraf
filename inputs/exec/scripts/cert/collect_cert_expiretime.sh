#!/bin/bash

# 脚本用途：检测网站证书过期时间

# 监控指标名
input_name=cert

# 自定义标签
cloud="huaweicloud"
region="huabei-beijing-4"
azone="az1"
product="cert"

# 需要被检测证书过期的域名
domain_list=(
www.baidu.com
www.weibo.com
www.csdn.net
)

function check_ssl() {
  domain=$1
  ts=$(date +%s)
  #localip=$(/usr/sbin/ifconfig `/usr/sbin/route | grep '^default' | awk '{print $NF}'` | grep inet | awk '{print $2}' | head -n 1)

  ping -c1 223.5.5.5 &> /dev/null
  if [ $? -eq 0 ];then
    END_TIME=$(echo | timeout 3 openssl s_client -servername ${domain} -connect "${domain}:443" 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | awk -F '=' '{print $2}' )
    END_TIME_STAMP=$(date +%s -d "${END_TIME}")
    NOW_TIME__STAMP=$(date +%s)
    ssl_expire_days=$(($((${END_TIME_STAMP} - ${NOW_TIME__STAMP}))/(60*60*24)))
    metrics="${input_name},cloud=${cloud},region=${region},azone=${azone},product=${product},domain_name=${domain} expire_days=${ssl_expire_days}"
    echo $metrics
  else
    pass
  fi
}

for i in ${domain_list[*]}
do
  data=$(check_ssl ${i})
  echo ${data}
done
