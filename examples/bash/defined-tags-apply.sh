#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <compartment-ocid>" >&2
  exit 2
fi

go run . \
  -compartment-id "$1" \
  -defined-tags '{"Operations":{"CostCenter":"42","Environment":"Production"},"Security":{"Classification":"Internal"}}' \
  -apply
