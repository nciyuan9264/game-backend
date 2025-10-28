#!/bin/bash

echo "🚀 开始生成所有服务的 gRPC 代码..."

# 为 room-service 生成 proto 代码
echo "📦 生成 room-service proto 代码..."
protoc --go_out=./services/room-service --go_opt=paths=source_relative \
       --go-grpc_out=./services/room-service --go-grpc_opt=paths=source_relative \
       shared/proto/room.proto

# 为 acquire 服务生成 proto 代码
echo "📦 生成 acquire 服务 proto 代码..."
cd services/acquire
mkdir -p proto
rm -f proto/*.pb.go
protoc --proto_path=../../shared/proto --go_out=./proto --go_opt=paths=source_relative \
       --go-grpc_out=./proto --go-grpc_opt=paths=source_relative \
       room.proto
cd ../..

# 为 splendor 服务生成 proto 代码
echo "📦 生成 splendor 服务 proto 代码..."
cd services/splendor
mkdir -p proto
rm -f proto/*.pb.go
protoc --proto_path=../../shared/proto --go_out=./proto --go_opt=paths=source_relative \
       --go-grpc_out=./proto --go-grpc_opt=paths=source_relative \
       room.proto
cd ../..

echo "✅ 所有服务的 gRPC 代码生成完成！"
echo "📋 生成的文件列表："
echo "  - services/room-service/proto/room.pb.go"
echo "  - services/room-service/proto/room_grpc.pb.go"
echo "  - services/acquire/proto/room.pb.go"
echo "  - services/acquire/proto/room_grpc.pb.go"
echo "  - services/splendor/proto/room.pb.go"
echo "  - services/splendor/proto/room_grpc.pb.go"