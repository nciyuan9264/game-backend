package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"room-service/config"

	"github.com/go-redis/redis/v8"
)

type RedisRepository struct {
	client *redis.Client
	ctx    context.Context
}

type RoomInfo struct {
	RoomID     string `json:"room_id"`
	UserID     string `json:"user_id"`
	GameType   string `json:"game_type"`
	MaxPlayers int    `json:"max_players"`
	RoomStatus bool   `json:"room_status"`
	GameStatus string `json:"game_status"`
}

type RoomPlayer struct {
	PlayerID string `json:"player_id"`
	Online   bool   `json:"online"`
}

func NewRedisRepository(cfg *config.Config) (*RedisRepository, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: "",
		DB:       cfg.RedisDB,
	})

	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	log.Println("✅ Redis 连接成功")
	return &RedisRepository{
		client: client,
		ctx:    ctx,
	}, nil
}

func (r *RedisRepository) CreateRoom(gameType, userID string, maxPlayers int) (string, error) {
	// 生成房间ID
	timePrefix := time.Now().Format("0102_150405")
	randomSuffix := r.generateRandomString(4)
	roomID := fmt.Sprintf("%s_%s", timePrefix, randomSuffix)

	// 设置房间信息
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomData := map[string]interface{}{
		"gameType":   gameType,
		"userID":     userID,
		"maxPlayers": maxPlayers,
		"roomStatus": false,
		"gameStatus": "waiting",
	}

	if err := r.client.HSet(r.ctx, roomKey, roomData).Err(); err != nil {
		return "", fmt.Errorf("创建房间失败: %w", err)
	}

	// 添加到房间列表
	roomListKey := fmt.Sprintf("rooms:%s", gameType)
	r.client.SAdd(r.ctx, roomListKey, roomID)
	r.client.SAdd(r.ctx, "rooms:all", roomID)

	return roomID, nil
}

func (r *RedisRepository) GetRoom(roomID string) (*RoomInfo, error) {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomInfoMap, err := r.client.HGetAll(r.ctx, roomKey).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %w", err)
	}
	if len(roomInfoMap) == 0 {
		return nil, fmt.Errorf("房间不存在")
	}

	roomInfo := &RoomInfo{
		RoomID:     roomID,
		UserID:     roomInfoMap["userID"],
		GameType:   roomInfoMap["gameType"],
		GameStatus: roomInfoMap["gameStatus"],
	}

	if maxPlayersStr := roomInfoMap["maxPlayers"]; maxPlayersStr != "" {
		if val, err := strconv.Atoi(maxPlayersStr); err == nil {
			roomInfo.MaxPlayers = val
		}
	}

	if roomStatusStr := roomInfoMap["roomStatus"]; roomStatusStr != "" {
		if val, err := strconv.ParseBool(roomStatusStr); err == nil {
			roomInfo.RoomStatus = val
		}
	}

	return roomInfo, nil
}

func (r *RedisRepository) UpdateRoomStatus(roomID string, roomStatus bool, gameStatus string) error {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	updateData := map[string]interface{}{
		"roomStatus": roomStatus,
	}
	if gameStatus != "" {
		updateData["gameStatus"] = gameStatus
	}

	return r.client.HSet(r.ctx, roomKey, updateData).Err()
}

func (r *RedisRepository) ListRooms(gameType string) ([]*RoomInfo, error) {
	var roomListKey string
	if gameType != "" {
		roomListKey = fmt.Sprintf("rooms:%s", gameType)
	} else {
		roomListKey = "rooms:all"
	}

	roomIDs, err := r.client.SMembers(r.ctx, roomListKey).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间列表失败: %w", err)
	}

	var rooms []*RoomInfo
	for _, roomID := range roomIDs {
		room, err := r.GetRoom(roomID)
		if err != nil {
			// 房间不存在，从列表中移除
			r.client.SRem(r.ctx, roomListKey, roomID)
			r.client.SRem(r.ctx, "rooms:all", roomID)
			continue
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func (r *RedisRepository) DeleteRoom(roomID string) error {
	// 获取房间信息以确定游戏类型
	room, err := r.GetRoom(roomID)
	if err != nil {
		return fmt.Errorf("房间不存在: %w", err)
	}

	// 删除所有房间相关的键
	prefix := fmt.Sprintf("room:%s:", roomID)
	var cursor uint64
	var keysToDelete []string

	for {
		keys, cur, err := r.client.Scan(r.ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return fmt.Errorf("扫描房间相关键失败: %w", err)
		}
		keysToDelete = append(keysToDelete, keys...)
		cursor = cur
		if cursor == 0 {
			break
		}
	}

	if len(keysToDelete) > 0 {
		if err := r.client.Del(r.ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("删除房间数据失败: %w", err)
		}
	}

	// 从房间列表中移除
	r.client.SRem(r.ctx, fmt.Sprintf("rooms:%s", room.GameType), roomID)
	r.client.SRem(r.ctx, "rooms:all", roomID)

	return nil
}

func (r *RedisRepository) JoinRoom(roomID, playerID string) error {
	playerKey := fmt.Sprintf("room:%s:players", roomID)
	playerData := RoomPlayer{
		PlayerID: playerID,
		Online:   true,
	}

	playerJSON, err := json.Marshal(playerData)
	if err != nil {
		return fmt.Errorf("序列化玩家数据失败: %w", err)
	}

	return r.client.HSet(r.ctx, playerKey, playerID, playerJSON).Err()
}

func (r *RedisRepository) LeaveRoom(roomID, playerID string) error {
	playerKey := fmt.Sprintf("room:%s:players", roomID)
	return r.client.HDel(r.ctx, playerKey, playerID).Err()
}

func (r *RedisRepository) GetRoomPlayers(roomID string) ([]*RoomPlayer, error) {
	playerKey := fmt.Sprintf("room:%s:players", roomID)
	playersData, err := r.client.HGetAll(r.ctx, playerKey).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间玩家失败: %w", err)
	}

	var players []*RoomPlayer
	for _, playerJSON := range playersData {
		var player RoomPlayer
		if err := json.Unmarshal([]byte(playerJSON), &player); err != nil {
			continue
		}
		players = append(players, &player)
	}

	return players, nil
}

func (r *RedisRepository) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
