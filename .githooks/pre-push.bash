#!/usr/bin/env bash

set -euo pipefail

# Check if we actually have commits to push
commits=$(git log @{u}..)
if [ -z "$commits" ]; then
    exit 0
fi

make build lint vet test-race