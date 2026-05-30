# HermesDeck 部署指南

## 前提条件
- Go >= 1.22
- Python >= 3.11
- Docker >= 24.0 (可选)

## Docker Compose（推荐）
```bash
make up
```

## 单体开发模式
```bash
# 1. 生成 gRPC 代码
./scripts/gen-proto.sh

# 2. 构建 Go Runtime
make build-go

# 3. 运行
./scripts/run-dev.sh
```

## 验证
- Web 界面: http://localhost:8080
- gRPC 健康检查: `grpcurl -plaintext localhost:50051 hermesdeck.HermesDeckBridge/HealthCheck`
