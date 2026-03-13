#!/usr/bin/env bash
# protogen.sh — Generate Go code from proto definitions.
#
# Usage: scripts/protogen.sh
#
# Requires: protoc, protoc-gen-go

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PROTO_PATH="proto"
GO_OUT="gen/go"
INCLUDE_PATH="${HOME}/.local/include"

mkdir -p "$GO_OUT"

PROTOS=(
  "proto/cog/reasoning/v1/reasoning.proto"
  "proto/cerebro/v1/cerebro.proto"
)

echo "Generating Go code from protos..."

protoc \
  --proto_path="$PROTO_PATH" \
  --proto_path="$INCLUDE_PATH" \
  --go_out="$GO_OUT" \
  --go_opt=paths=source_relative \
  "${PROTOS[@]}"

echo "Done. Generated files:"
find "$GO_OUT" -name '*.pb.go' -printf '  %p\n'
