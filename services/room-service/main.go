package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"room-service/config"
	pb "room-service/proto"
	"room-service/repository"
	"room-service/server"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()
	log.Printf("启动房间服务，配置: Redis=%s, gRPC端口=%s", cfg.RedisAddr, cfg.GRPCPort)

	// 初始化 Redis 仓储
	repo, err := repository.NewRedisRepository(cfg)
	if err != nil {
		log.Fatalf("初始化 Redis 仓储失败: %v", err)
	}

	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer()
	roomServer := server.NewRoomServer(repo)

	// 注册服务
	pb.RegisterRoomServiceServer(grpcServer, roomServer)

	// 启用反射（用于调试）
	reflection.Register(grpcServer)

	// 监听端口
	lis, err := net.Listen("tcp", cfg.GRPCPort)
	if err != nil {
		log.Fatalf("监听端口失败: %v", err)
	}

	log.Printf("🚀 房间服务启动成功，监听端口: %s", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("启动 gRPC 服务失败: %v", err)
	}
}
