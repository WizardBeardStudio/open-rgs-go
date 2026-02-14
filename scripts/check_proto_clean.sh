#!/usr/bin/env bash
set -euo pipefail

buf lint
buf generate

if ! git diff --quiet -- gen/rgs/v1; then
  echo "generated protobuf artifacts are out of date; run 'buf generate' and commit changes" >&2
  git --no-pager diff --stat -- gen/rgs/v1
  exit 1
fi

echo "proto check passed: generated artifacts are up to date"
