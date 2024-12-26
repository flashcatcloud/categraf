#!/bin/sh
set -e

    # Allow caegraf to send ICMP packets and bind to privliged ports
    setcap cap_net_raw,cap_net_bind_service+ep /usr/bin/categraf || echo "Failed to set additional capabilities on /usr/bin/categraf"

    if [ $N9E_HOST ];then
        sed -i "s/127.0.0.1:17000/$N9E_HOST/g" /etc/categraf/conf/config.toml
    fi
    exec /usr/bin/categraf -configs=/etc/categraf/conf
