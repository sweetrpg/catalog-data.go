#!/bin/bash

set -e

export $(cat "$(dirname "$0")/../.common-env" | xargs)
export $(cat "$(dirname "$0")/../.local-env" | xargs)

go test -v ./... \
  -run TestLocal \
  -tags=local \
  -timeout=30s \
  -coverprofile=coverage.out \
  -covermode=atomic
