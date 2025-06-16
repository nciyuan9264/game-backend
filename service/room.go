package service

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"go-game/ws"
	"strings"

	"github.com/google/uuid"
)

func CreateRoom(params dto.CreateRoomRequest) (string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 生成唯一 Room ID（例如 8位）
	uuidStr := uuid.New().String()
	roomID := strings.ReplaceAll(uuidStr, "-", "")[:8]

	// 初始化房间信息
	err := ws.SetRoomInfo(rdb, repository.Ctx, roomID, entities.RoomInfo{
		MaxPlayers: params.MaxPlayers,
		GameStatus: dto.RoomStatusSetTile,
		RoomStatus: false,
		UserID:     params.UserID,
	})
	if err != nil {
		return "", fmt.Errorf("初始化房间信息失败: %w", err)
	}

	companyData := map[string]map[string]interface{}{
		"Sackson": {
			"name":       "Sackson",
			"stockTotal": 25,
			"tiles":      0,   // 初始数量
			"stockPrice": 200, // 初始参考股价（可调整）
		},
		"Tower": {
			"name":       "Tower",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"American": {
			"name":       "American",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Festival": {
			"name":       "Festival",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Worldwide": {
			"name":       "Worldwide",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Continental": {
			"name":       "Continental",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Imperial": {
			"name":       "Imperial",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
	}

	for id, data := range companyData {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, id)
		if _, err := rdb.HSet(ctx, companyKey, data).Result(); err != nil {
			return "", fmt.Errorf("初始化公司[%s]失败: %w", id, err)
		}
		rdb.SAdd(ctx, fmt.Sprintf("room:%s:company_ids", roomID), id)
	}

	tileKey := fmt.Sprintf("room:%s:tiles", roomID)
	pipe := rdb.Pipeline()

	for col := 1; col <= 12; col++ {
		for row := 'A'; row <= 'I'; row++ {
			id := fmt.Sprintf("%d%c", col, row)
			tile := dto.Tile{
				ID:     id,
				Belong: "",
			}
			tileJSON, err := json.Marshal(tile)
			if err != nil {
				return "", fmt.Errorf("tile %s 序列化失败: %w", id, err)
			}
			pipe.HSet(ctx, tileKey, id, tileJSON)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("tile 初始化 Redis 写入失败: %w", err)
	}

	ws.Rooms[roomID] = []dto.PlayerConn{}
	return roomID, nil
}

func DeleteRoom(params dto.DeleteRoomRequest) error {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 用 SCAN 查找所有以 room:{RoomID}: 开头的 key
	prefix := fmt.Sprintf("room:%s:", params.RoomID)
	var cursor uint64
	var keysToDelete []string

	for {
		keys, cur, err := rdb.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return fmt.Errorf("扫描房间相关 key 失败: %w", err)
		}
		keysToDelete = append(keysToDelete, keys...)
		cursor = cur
		if cursor == 0 {
			break
		}
	}

	if len(keysToDelete) == 0 {
		return fmt.Errorf("房间不存在或无相关数据")
	}

	// 批量删除这些 key
	if _, err := rdb.Del(ctx, keysToDelete...).Result(); err != nil {
		return fmt.Errorf("删除房间相关 key 失败: %w", err)
	}
	delete(ws.Rooms, params.RoomID)

	return nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	rdb := repository.Rdb
	var rooms []dto.RoomInfo
	for roomID, roomConnInfo := range ws.Rooms {
		roomPlayers := make([]dto.RoomPlayer, 0, len(roomConnInfo))
		for _, player := range roomConnInfo {
			roomPlayers = append(roomPlayers, dto.RoomPlayer{
				PlayerID: player.PlayerID,
				Online:   player.Online,
			})
		}

		roomInfo, err := ws.GetRoomInfo(rdb, roomID)
		if err != nil {
			delete(ws.Rooms, roomID)
			continue
		}
		room := dto.RoomInfo{
			RoomID:     roomID,
			UserID:     roomInfo.UserID,
			MaxPlayers: roomInfo.MaxPlayers,
			Status:     roomInfo.RoomStatus,
			RoomPlayer: roomPlayers,
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func GetOnlinePlayer() (int, error) {
	onlinePlayer := 0
	for _, room := range ws.Rooms {
		for _, player := range room {
			if player.Online {
				onlinePlayer++
			}
		}
	}
	return onlinePlayer, nil
}
