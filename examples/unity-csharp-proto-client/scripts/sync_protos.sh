#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
example_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

rgs_proto_src="${root}/api/proto/rgs/v1"
rgs_proto_dst="${example_dir}/Protos/rgs/v1"
google_api_dst="${example_dir}/Protos/google/api"

mkdir -p "${rgs_proto_dst}" "${google_api_dst}"
cp "${rgs_proto_src}"/*.proto "${rgs_proto_dst}/"

if ! command -v buf >/dev/null 2>&1; then
  echo "buf is required to export google/api protos for this example." >&2
  echo "Install buf and rerun: https://buf.build/docs/cli/installation/" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

if ! buf export buf.build/googleapis/googleapis --path google/api -o "${tmpdir}"; then
  echo "failed to export googleapis protos with buf." >&2
  echo "check network access and rerun." >&2
  exit 1
fi

cp "${tmpdir}/google/api/"*.proto "${google_api_dst}/"

cat <<EOF
synced proto sources:
  rgs: ${rgs_proto_dst}
  google api: ${google_api_dst}
EOF
