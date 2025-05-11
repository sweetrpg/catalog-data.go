#!/bin/bash

set -e

export $(cat "$(dirname "$0")/../.common-env" | xargs)
export $(cat "$(dirname "$0")/../.atlas-env" | xargs)

go test -v ./... \
  -timeout=30s \
  -coverprofile=coverage.out \
  -covermode=atomic
