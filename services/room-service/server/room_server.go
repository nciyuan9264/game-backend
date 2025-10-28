package server

import (
	"context"
	"fmt"
	"log"

	pb "room-service/proto"
	"room-service/repository"
)

type RoomServer struct {
	pb.UnimplementedRoomServiceServer
	repo *repository.RedisRepository
}

func NewRoomServer(repo *repository.RedisRepository) *RoomServer {
	return &RoomServer{
		repo: repo,
	}
}

func (s *RoomServer) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.CreateRoomResponse, error) {
	log.Printf("创建房间请求: gameType=%s, maxPlayers=%d, userID=%s", req.GameType, req.MaxPlayers, req.UserId)

	roomID, err := s.repo.CreateRoom(req.GameType, req.UserId, int(req.MaxPlayers))
	if err != nil {
		log.Printf("创建房间失败: %v", err)
		return &pb.CreateRoomResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.CreateRoomResponse{
		RoomId:  roomID,
		Success: true,
		Message: "房间创建成功",
	}, nil
}

func (s *RoomServer) ListRooms(ctx context.Context, req *pb.ListRoomsRequest) (*pb.ListRoomsResponse, error) {
	rooms, err := s.repo.ListRooms(req.GameType)
	if err != nil {
		return &pb.ListRoomsResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	log.Printf("获取房间列表成功1: %v", rooms)

	// 注意：这里需要根据实际的proto定义调整
	// 如果ListRoomsResponse没有Rooms字段，可能需要修改proto文件
	pbRooms := make([]*pb.RoomInfo, len(rooms))
	for i, room := range rooms {
		// 获取每个房间的玩家信息
		players, _ := s.repo.GetRoomPlayers(room.RoomID)
		pbPlayers := make([]*pb.RoomPlayer, len(players))
		for j, player := range players {
			pbPlayers[j] = &pb.RoomPlayer{
				PlayerId: player.PlayerID,
				Online:   player.Online,
			}
		}

		pbRooms[i] = &pb.RoomInfo{
			RoomId:     room.RoomID,
			UserId:     room.UserID,
			MaxPlayers: int32(room.MaxPlayers),
			Status:     room.RoomStatus, // 使用Status而不是RoomStatus
			Players:    pbPlayers,
		}
	}
	fmt.Printf("获取房间列表成功2: %v", pbRooms)

	return &pb.ListRoomsResponse{
		Rooms:   pbRooms, // 需要确认proto中是否有这个字段
		Success: true,
		Message: "获取房间列表成功1",
	}, nil
}

func (s *RoomServer) DeleteRoom(ctx context.Context, req *pb.DeleteRoomRequest) (*pb.DeleteRoomResponse, error) {
	err := s.repo.DeleteRoom(req.RoomId)
	if err != nil {
		return &pb.DeleteRoomResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.DeleteRoomResponse{
		Success: true,
		Message: "删除房间成功",
	}, nil
}
