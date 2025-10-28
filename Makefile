.PHONY: proto build clean run-room run-acquire run-splendor docker-up docker-down

# 生成 proto 文件
proto:
	@echo "生成 proto 文件..."
	@chmod +x scripts/generate_proto.sh
	@./scripts/generate_proto.sh

# 构建所有服务
build: proto
	@echo "构建所有服务..."
	@cd services/room-service && go build -o ../../bin/room-service .
	@cd services/acquire && go build -o ../../bin/acquire .
	@cd services/splendor && go build -o ../../bin/splendor .

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf bin/
	@rm -f services/*/proto/*.pb.go

# 运行服务
run-room:
	@echo "启动 room-service..."
	@cd services/room-service && go run .

run-acquire:
	@echo "启动 acquire 服务..."
	@cd services/acquire && go run .

run-splendor:
	@echo "启动 splendor 服务..."
	@cd services/splendor && go run .

# Docker 操作
docker-up:
	@echo "启动 Docker 服务..."
	@cd infrastructure && docker-compose up -d

docker-down:
	@echo "停止 Docker 服务..."
	@cd infrastructure && docker-compose down

# 开发环境启动
dev: docker-up
	@echo "启动开发环境..."
	@echo "请在不同终端中运行："
	@echo "  make run-room"
	@echo "  make run-acquire"
	@echo "  make run-splendor"

# 帮助信息
help:
	@echo "可用命令："
	@echo "  proto        - 生成 proto 文件"
	@echo "  build        - 构建所有服务"
	@echo "  clean        - 清理构建文件"
	@echo "  run-room     - 运行 room-service"
	@echo "  run-acquire  - 运行 acquire 服务"
	@echo "  run-splendor - 运行 splendor 服务"
	@echo "  docker-up    - 启动 Docker 服务"
	@echo "  docker-down  - 停止 Docker 服务"
	@echo "  dev          - 启动开发环境"