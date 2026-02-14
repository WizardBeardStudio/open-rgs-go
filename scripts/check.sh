#!/usr/bin/env bash
set -euo pipefail
make proto-check
make fmt
make test
