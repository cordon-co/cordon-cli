#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../cli"
go test ./... -count=1 -v "$@"
