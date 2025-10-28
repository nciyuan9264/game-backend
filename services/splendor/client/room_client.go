package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// 使用 splendor 自己的 proto
	roomproto "splendor-service/proto"
)

type RoomClient struct {
	conn   *grpc.ClientConn
	client roomproto.RoomServiceClient
}

var RoomServiceClient *RoomClient

func InitClients() error {
	// 连接到 room-service
	roomClient, err := NewRoomClient("localhost:50051") // 根据实际地址调整
	if err != nil {
		return fmt.Errorf("初始化房间客户端失败: %w", err)
	}
	RoomServiceClient = roomClient
	return nil
}

func NewRoomClient(address string) (*RoomClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("连接 room-service 失败: %w", err)
	}

	client := roomproto.NewRoomServiceClient(conn)
	return &RoomClient{
		conn:   conn,
		client: client,
	}, nil
}

func (rc *RoomClient) Close() error {
	return rc.conn.Close()
}

// 创建房间
func (rc *RoomClient) CreateRoom(ctx context.Context, gameType string, maxPlayers int32, aiCount int32, userID string) (*roomproto.CreateRoomResponse, error) {
	req := &roomproto.CreateRoomRequest{
		GameType:   gameType,
		MaxPlayers: maxPlayers,
		AiCount:    aiCount,
		UserId:     userID,
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return rc.client.CreateRoom(ctx, req)
}

// 删除房间
func (rc *RoomClient) DeleteRoom(ctx context.Context, roomID string) (*roomproto.DeleteRoomResponse, error) {
	req := &roomproto.DeleteRoomRequest{
		RoomId: roomID,
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return rc.client.DeleteRoom(ctx, req)
}

// 获取房间列表
func (rc *RoomClient) ListRooms(ctx context.Context, gameType string) (*roomproto.ListRoomsResponse, error) {
	req := &roomproto.ListRoomsRequest{
		GameType: gameType,
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return rc.client.ListRooms(ctx, req)
}