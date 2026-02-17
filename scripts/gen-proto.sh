#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="$ROOT_DIR/proto"
OUT_GO="$PROTO_DIR/gen/go"
OUT_TS="$ROOT_DIR/gateway-mcp/src/gen"

mkdir -p "$OUT_GO" "$OUT_TS"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$OUT_GO" --go_opt=module=github.com/fast-mobile-mcp/proto/gen/go \
  --go-grpc_out="$OUT_GO" --go-grpc_opt=module=github.com/fast-mobile-mcp/proto/gen/go \
  "$PROTO_DIR/mobile.proto"

npx protoc \
  --proto_path="$PROTO_DIR" \
  --plugin=protoc-gen-ts_proto=./gateway-mcp/node_modules/.bin/protoc-gen-ts_proto \
  --ts_proto_out="$OUT_TS" \
  --ts_proto_opt=outputServices=grpc-js,esModuleInterop=true,forceLong=string,importSuffix=.js \
  "$PROTO_DIR/mobile.proto"
