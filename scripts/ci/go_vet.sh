#!/usr/bin/env bash

set -euo pipefail

go list ./... | grep -vE "/inputs/mtail/internal/runtime/compiler/parser|pkg/otel/fanoutconsumer|pkg/otel/pipelines|agent/install|agent/update" | xargs go vet -tests=false
