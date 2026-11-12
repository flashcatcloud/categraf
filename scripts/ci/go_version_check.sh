#!/usr/bin/env bash

set -euo pipefail

# Keep the CI minimum synchronized with the module and compare numeric versions.
# Comparing an unstripped "go1.x" against "1.x" always accepted older toolchains.
required_version=$(awk '$1 == "go" { print $2; exit }' go.mod)
current_version=$(go env GOVERSION)
normalized_version=${current_version#devel }
normalized_version=${normalized_version#go}

if [[ -z "$required_version" || ! "$normalized_version" =~ ^[0-9]+\.[0-9]+ ]]; then
    >&2 echo "Error: cannot compare Go version $current_version against go.mod"
    exit 1
fi

if ! awk -v required="$required_version" -v actual="$normalized_version" 'BEGIN {
    split(required, r, ".")
    split(actual, a, ".")
    for (i = 1; i <= 3; i++) {
        if (a[i] + 0 < r[i] + 0) exit 1
        if (a[i] + 0 > r[i] + 0) exit 0
    }
    # A prerelease of the required version precedes its stable release.
    if (actual ~ /(beta|rc|-)/) exit 1
}'; then
    >&2 echo "Error: Go $required_version or higher is required, but found $current_version"
    exit 1
fi
