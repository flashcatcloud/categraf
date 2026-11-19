#!/usr/bin/env bash

set -euo pipefail

# Bound both the number of builds and each Go command's package parallelism.
# Running several full Categraf builds at once can exhaust a hosted runner.
export GOMAXPROCS=2

report_resources() {
    printf '\n--- Release resources: %s ---\n' "$(date -u +%FT%TZ)"
    free -m
    df -h . /tmp
    # Print process names only; command-line arguments can contain credentials.
    ps -eo pid,ppid,rss,comm --sort=-rss | head -n 12
}

monitor_resources() {
    local sleep_pid=''
    trap 'if [[ -n "$sleep_pid" ]]; then kill "$sleep_pid" 2>/dev/null || true; wait "$sleep_pid" 2>/dev/null || true; fi; exit 0' TERM INT
    while true; do
        report_resources || true
        sleep 30 &
        sleep_pid=$!
        wait "$sleep_pid"
    done
}

monitor_resources &
monitor_pid=$!
cleanup() {
    # Stop the monitor's sleep as well as its shell, and preserve the build status.
    kill "$monitor_pid" 2>/dev/null || true
    wait "$monitor_pid" 2>/dev/null || true
    report_resources || true
}
trap cleanup EXIT

goreleaser "$@" --parallelism 1
