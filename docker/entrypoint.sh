#!/bin/sh
set -e

    # Allow caegraf to send ICMP packets and bind to privliged ports
    setcap cap_net_raw,cap_net_bind_service+ep /usr/bin/categraf || echo "Failed to set additional capabilities on /usr/bin/categraf"

    exec /usr/bin/categraf -configs=/etc/categraf/conf
