package service

import (
	"context"
	// 使用 splendor 自己的 client
	client "splendor-service/client"
	roomproto "splendor-service/proto"
)

func GetRoomList(gameType string) ([]*roomproto.RoomInfo, error) {
	ctx := context.Background()

	// 使用 splendor 自己的 RoomClient
	resp, err := client.RoomServiceClient.ListRooms(ctx, gameType)
	if err != nil {
		return nil, err
	}

	// 现在类型完全匹配了
	return resp.Rooms, nil
}

func convertPlayers(protoPlayers []*roomproto.RoomPlayer) []*roomproto.RoomPlayer {
	// 直接返回原始的指针切片，因为类型已经匹配
	return protoPlayers
}
