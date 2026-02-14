#!/usr/bin/env bash
set -euo pipefail

mode="${RGS_PROTO_CHECK_MODE:-full}"

case "${mode}" in
  full)
    if ! buf lint; then
      echo "buf lint failed in full mode; if running in an offline/local-restricted environment, retry with RGS_PROTO_CHECK_MODE=diff-only" >&2
      exit 1
    fi
    if ! buf generate; then
      echo "buf generate failed in full mode; if running in an offline/local-restricted environment, retry with RGS_PROTO_CHECK_MODE=diff-only" >&2
      exit 1
    fi
    ;;
  diff-only)
    echo "proto check running in diff-only mode (skipping buf lint/generate)"
    ;;
  *)
    echo "invalid RGS_PROTO_CHECK_MODE=${mode}; expected 'full' or 'diff-only'" >&2
    exit 1
    ;;
esac

if ! git diff --quiet -- gen/rgs/v1; then
  echo "generated protobuf artifacts are out of date; run 'buf generate' and commit changes" >&2
  git --no-pager diff --stat -- gen/rgs/v1
  exit 1
fi

echo "proto check passed: generated artifacts are up to date (${mode})"
