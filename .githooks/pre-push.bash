#!/usr/bin/env bash

set -euo pipefail

# Get current branch name
current=$(git rev-parse --abbrev-ref HEAD)
if git branch -r | grep "^  ${1}/${current}$" &> /dev/null; then
    # Check if we actually have commits to push
    commits=$(git log @{u}..)
    if [ -z "$commits" ]; then
        exit 0
    fi
fi

make build lint vet test-race