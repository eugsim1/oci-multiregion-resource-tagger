#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <compartment-ocid>" >&2
  exit 2
fi

go run . \
  -compartment-id "$1" \
  -freeform-tags '{"Environment":"Production","Owner":"FinOps"}'
