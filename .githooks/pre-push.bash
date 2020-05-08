#!/usr/bin/env bash

set -euo pipefail

# Check if we actually have commits to push
commits=$(git log @{u}..)
if [ -z "$commits" ]; then
    exit 0
fi

if ! command -v golangci-lint &> /dev/null; then
    echo "installing golangci-lint..."
    make linter
fi

make lint vet test-race
