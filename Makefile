.PHONY: all proto build-go build-py build dev up down clean deploy

# === 全局变量 ===
GO_CMD      ?= go
PYTHON_CMD  ?= python3
PROTO_DIR   := proto
GO_OUT      := src/go/pkg/proto
PY_OUT      := src/python/hermesdeck_sidecar/proto
TARGET_DIR  := /home/ts/HermesDeck

# ============================================================
# 默认目标
# ============================================================
all: proto build

# ============================================================
# 生成 gRPC 代码
# ============================================================
proto: $(PROTO_DIR)/hermesdeck.proto
	@echo ">>> Generating gRPC code..."
	mkdir -p $(GO_OUT) $(PY_OUT)
	protoc --go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		--python_out=$(PY_OUT) \
		--grpc_python_out=$(PY_OUT) \
		--proto_path=$(PROTO_DIR) \
		$(PROTO_DIR)/hermesdeck.proto
	@echo ">>> gRPC code generated."

# ============================================================
# 构建
# ============================================================
build-go:
	@echo ">>> Building Go Runtime..."
	cd src/go && $(GO_CMD) build -o ../../bin/hermesdeck ./cmd/hermesdeck/
	@echo ">>> Go Runtime built: bin/hermesdeck"

build-py:
	@echo ">>> Python Sidecar is interpreted; skipping build."
	@echo ">>> Dependencies: cd src/python && pip install -r requirements.txt"

build: build-go

# ============================================================
# 运行
# ============================================================
dev: build
	@echo ">>> Starting HermesDeck in dev mode..."
	./scripts/run-dev.sh

# ============================================================
# Docker
# ============================================================
up:
	@echo ">>> Starting HermesDeck with Docker Compose..."
	docker-compose up --build

down:
	docker-compose down

# ============================================================
# 部署到目标目录
# ============================================================
deploy:
	@echo ">>> Deploying to $(TARGET_DIR)..."
	mkdir -p $(TARGET_DIR)
	rsync -av --exclude='.git' --exclude='bin/' ./ $(TARGET_DIR)/
	@echo ">>> Deployed to $(TARGET_DIR)"

# ============================================================
# 清理
# ============================================================
clean:
	rm -rf bin/
	rm -rf $(GO_OUT)/*
	rm -rf $(PY_OUT)/*.py
	@echo ">>> Cleaned."

# ============================================================
# 测试
# ============================================================
test-go:
	cd src/go && $(GO_CMD) test ./... -v

test-py:
	cd src/python && $(PYTHON_CMD) -m pytest -v

test: test-go test-py
