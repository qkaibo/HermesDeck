#!/usr/bin/env bash
# ============================================================
# HermesDeck — gRPC 代码生成脚本
# 用法: ./scripts/gen-proto.sh
# 前提: 需要安装 protoc + Go 插件 + Python 插件
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

PROTO_DIR="$PROJECT_DIR/proto"
GO_OUT="$PROJECT_DIR/src/go/pkg/proto"
PY_OUT="$PROJECT_DIR/src/python/hermesdeck_sidecar/proto"

echo ">>> HermesDeck Proto Generator"
echo "    Proto dir:  $PROTO_DIR"
echo "    Go out:     $GO_OUT"
echo "    Python out: $PY_OUT"

# 检查依赖
command -v protoc >/dev/null 2>&1 || { echo "ERROR: protoc not found. Install it first."; exit 1; }

# 创建输出目录
mkdir -p "$GO_OUT" "$PY_OUT"

# 生成 Go 代码
echo ">>> Generating Go gRPC code..."
protoc --go_out="$GO_OUT" --go_opt=paths=source_relative \
    --go-grpc_out="$GO_OUT" --go-grpc_opt=paths=source_relative \
    --proto_path="$PROTO_DIR" \
    "$PROTO_DIR"/hermesdeck.proto

# 生成 Python 代码
echo ">>> Generating Python gRPC code..."
protoc --python_out="$PY_OUT" \
    --grpc_python_out="$PY_OUT" \
    --proto_path="$PROTO_DIR" \
    "$PROTO_DIR"/hermesdeck.proto

# 创建 Python 包 init 文件
touch "$PY_OUT/__init__.py"

echo ">>> Done! Generated files:"
echo "    Go:     $GO_OUT/"
ls -la "$GO_OUT/"*.go 2>/dev/null || echo "    (no .go files)"
echo "    Python: $PY_OUT/"
ls -la "$PY_OUT/"*.py 2>/dev/null || echo "    (no .py files)"
