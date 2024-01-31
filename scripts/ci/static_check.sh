#!/usr/bin/env bash

set -e

# static check docs: https://staticcheck.dev/docs/checks/

(
GOOS=linux  go list ./... | xargs staticcheck -go=1.21 -tests=false -f binary
GOOS=darwin go list ./... | xargs staticcheck -go=1.21 -tests=false -f binary
) | staticcheck -merge -f=text | tee static-check.log
lines=$(cat static-check.log | wc -l)
rm -f static-check.log
if [[ $lines -eq 0 ]]; then
  echo "static check pass"
  exit 0
else
  echo "static check failed"
  exit 1
fi