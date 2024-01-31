#!/usr/bin/env bash

set -eo pipefail

function main () {
    local -r required_version="1.21"  # 设置所需的最低 Go 版本

    local -r current_version=$(go version | awk '{print $3}')  # 获取当前 Go 版本

    if [[ "$(printf '%s\n' "$required_version" "$current_version" | sort -V | head -n1)" != "$required_version" ]]; then
        >&2 echo "Error: Go version $required_version or higher is required, but found $current_version"
        exit 1
    fi
}

main