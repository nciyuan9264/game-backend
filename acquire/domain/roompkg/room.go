package roompkg

import (
	"encoding/json"
	"fmt"
	"time"

	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/domain/game"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
)

type RoomService struct {
	Room *domain.Room
}

func (r *RoomService) Run() {
	for {
		select {
		case cmd := <-r.Room.CmdCh:
			r.handleCommand(cmd)
			if r.Room.Status == dto.RoomStatusMatch {
				game.BroadcastToMatch(r.Room)
			} else {
				game.BroadcastToRoom(r.Room)
			}
		case <-r.Room.QuitCh:
			return
		}
	}
}

const MaxPlayers = 6

var Rooms = map[string]*RoomService{}
var RoomLock sync.Mutex

func transferOwnerOrDelete(r *domain.Room) bool {
	for _, p := range r.Players {
		if !p.AI && p.Online {
			r.OwnerID = p.PlayerID
			return false
		}
	}

	// 没有在线真人了
	delete(Rooms, r.ID)
	return true
}

func MarkPlayerOffline(r *domain.Room, playerID string) (roomDeleted bool) {
	var ownerLeft bool

	if r.Status == dto.RoomStatusMatch {
		for i, pID := range r.PlayerSeq {
			if pID == playerID {
				r.PlayerSeq = append(r.PlayerSeq[:i], r.PlayerSeq[i+1:]...)
				break
			}
		}
		delete(r.Players, playerID)
		if playerID == r.OwnerID {
			ownerLeft = true
		}
		log.Printf("玩家 %s 已从匹配队列中移除\n", playerID)
	} else {
		p, ok := r.Players[playerID]
		if !ok {
			return
		}

		p.Online = false
		p.Conn = nil

		log.Printf("玩家 %s 标记为离线\n", playerID)
	}

	log.Printf("玩家 %s 是否是房主: %v\n", playerID, ownerLeft)
	if ownerLeft {
		return transferOwnerOrDelete(r)
	}

	return false
}

// func handleOwnerLeave(r *roompkg.Room) {
// 	// 找第一个非 AI 玩家
// 	for _, p := range r.Players {
// 		if !p.AI {
// 			r.OwnerID = p.PlayerID
// 			return
// 		}
// 	}

// 	// 没有真人了 → 删除房间
// 	delete(room.Rooms, r.ID)
// }

// 玩家断开连接后，从房间中移除该连接
func handleDisconnectCommand(r *domain.Room, cmd domain.Command) {
	// 1️⃣ 通知 room：这个人掉线了
	roomDeleted := MarkPlayerOffline(r, cmd.PlayerID)

	// 2️⃣ 同步 Redis 房间状态（如果房间还在）
	if !roomDeleted && r.Status != dto.RoomStatusMatch {
		roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
		if err != nil {
			log.Println("❌ 获取房间信息失败:", err)
		} else if roomInfo.RoomStatus {
			data.SetRoomStatus(repository.Rdb, r.ID, false)
		}
	}
}

// 获取房间中玩家数量
func getRoomPlayerCount(room *domain.Room) int {
	onLineCount := 0
	for _, pc := range room.Players {
		if pc.Online {
			onLineCount++
		}
	}
	return onLineCount
}

func SetRoomStatusCache(roomID string, status dto.RoomStatus) error {
	// 1️⃣ 更新内存状态（权威）
	r, ok := Rooms[roomID]
	if !ok {
		return fmt.Errorf("room not found in memory: %s", roomID)
	}
	r.Room.Status = status
	return nil
}

// handleCommand 处理房间命令
func (r *RoomService) handleCommand(cmd domain.Command) {
	switch cmd.Type {
	case "connect":
		handleConnectCommand(r.Room, cmd)
	case "disconnect":
		handleDisconnectCommand(r.Room, cmd)
	case "match_ready":
		HandleMatchReady(r.Room, cmd)
	case "match_begin":
		HandleMatchBegin(r.Room, cmd)
	case "match_add_ai":
		HandleAddAI(r.Room, cmd)
	case "match_remove_player":
		HandleRemovePlayer(r.Room, cmd)
	case "game_ready":
		HandleReadyMessage(r.Room, cmd)
	case "game_place_tile":
		game.HandlePlaceTileMessage(r.Room, cmd)
	case "game_create_company":
		game.HandleCreateCompanyMessage(r.Room, cmd)
	case "game_merging_settle":
		game.HandleMergingSettleMessage(r.Room, cmd)
	case "game_buy_stock":
		game.HandleBuyStockMessage(r.Room, cmd)
	case "game_merging_selection":
		game.HandleMergingSelectionMessage(r.Room, cmd)
	case "game_end":
		game.HandleGameEndMessage(r.Room, cmd)
	case "game_play_audio":
		game.HandlePlayAudioMessage(r.Room, cmd)
	case "game_restart":
		game.HandleRestartGameMessage(r.Room, cmd)
	case "game_restart_game":
		game.HandleRestartGameMessage(r.Room, cmd)
	default:
		log.Printf("未知命令类型: %s", cmd.Type)
	}
}

// handleConnectCommand 处理玩家加入命令
func handleConnectCommand(r *domain.Room, cmd domain.Command) {
	playerID := cmd.PlayerID
	conn := cmd.Conn

	// 1️⃣ 已存在玩家（重连 or 重复）
	if p, ok := r.Players[playerID]; ok {
		log.Printf("玩家 %s 尝试加入房间 %s，当前状态：%v\n", playerID, r.ID, p.Online)
		if !p.Online {
			p.Conn = conn
			p.Online = true
			log.Printf("玩家 %s 重连成功\n", playerID)
			return
		}

		// 已在线，重复连接
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"PLAYER_ALREADY_IN_ROOM"}`))
		conn.Close()
		return
	}

	log.Printf("玩家 %s 尝试加入房间 %s（新玩家）\n", playerID, r.ID)

	// 2️⃣ 状态校验
	if r.Status != dto.RoomStatusMatch {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"游戏已开始，无法加入"}`))
		conn.Close()
		return
	}

	// 3️⃣ 人数校验
	if len(r.Players) >= MaxPlayers {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
		conn.Close()
		return
	}

	// 4️⃣ 新玩家加入
	r.Players[playerID] = &dto.PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
		Ready:    r.OwnerID == playerID,
		AI:       false,
	}
	r.PlayerSeq = append(r.PlayerSeq, playerID)

	log.Printf("玩家 %s 加入房间 %s\n", playerID, r.ID)
}

type ReadyPayload struct {
	Ready bool `json:"ready"`
}

func HandleMatchReady(r *domain.Room, cmd domain.Command) {
	var p ReadyPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}
	ready := p.Ready

	// 更新玩家准备状态
	for _, pc := range r.Players {
		if pc.PlayerID == cmd.PlayerID {
			pc.Ready = ready
			break
		}
	}
}

func HandleMatchBegin(r *domain.Room, cmd domain.Command) {
	ctx := repository.Ctx
	rdb := repository.Rdb
	// 检查是否所有玩家都已准备
	allReady := true
	for _, pc := range r.Players {
		if !pc.Ready {
			allReady = false
			break
		}
	}

	if !allReady {
		log.Println("❌ 不是所有玩家都已准备")
		return
	}

	// 初始化房间信息
	err := data.SetRoomInfo(rdb, repository.Ctx, r.ID, entities.RoomInfo{
		RoomStatus: false,
		GameStatus: dto.RoomStatusWaiting,
		MaxPlayers: len(r.Players),
		OwnerID:    r.OwnerID,
	})
	if err != nil {
		log.Printf("❌ 初始化房间信息失败: %v\n", err)
		return
	}

	// 初始化公司数据
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
		companyKey := fmt.Sprintf("room:%s:company:%s", r.ID, id)
		if _, err := rdb.HSet(ctx, companyKey, data).Result(); err != nil {
			log.Printf("❌ 初始化公司[%s]失败: %v\n", id, err)
			return
		}
		rdb.SAdd(ctx, fmt.Sprintf("room:%s:company_ids", r.ID), id)
	}

	// 初始化游戏棋盘（12x9 个 tile）
	tileKey := fmt.Sprintf("room:%s:tiles", r.ID)
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
				log.Printf("❌ tile %s 序列化失败: %v\n", id, err)
				return
			}
			pipe.HSet(ctx, tileKey, id, tileJSON)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("❌ tile 初始化 Redis 写入失败: %v\n", err)
		return
	}

	for playerID, player := range r.Players {
		if player.AI {
			data.InitPlayerData(r, playerID)
			log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, r.ID)
		}
	}

	// 更新房间状态为匹配中
	err = SetRoomStatusCache(r.ID, dto.RoomStatusWaiting)
	if err != nil {
		log.Printf("❌ 内存设置房间状态失败: %v\n", err)
		return
	}
	err = data.SetGameStatus(rdb, r.ID, dto.RoomStatusWaiting)
	if err != nil {
		log.Printf("❌ redis设置游戏状态失败: %v\n", err)
		return
	}

	// 开始游戏
	log.Println("所有玩家都已准备，开始游戏")
}

func JoinMatchAsAI(r *domain.Room, playerID string) bool {
	r.Players[playerID] = &dto.PlayerConn{
		PlayerID: playerID,
		Conn:     &VirtualConn{PlayerID: playerID, RoomID: r.ID},
		Online:   true,
		Ready:    true,
		AI:       true,
	}
	r.PlayerSeq = append(r.PlayerSeq, playerID)

	log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, r.ID)
	return true
}

func HandleAddAI(r *domain.Room, cmd domain.Command) {
	// 检查房间是否已满
	if len(r.Players) >= MaxPlayers {
		log.Println("❌ 房间已满，无法添加 AI")
		return
	}

	// 检查房间状态是否为等待加入
	if r.Status != dto.RoomStatusMatch {
		log.Println("❌ 房间状态不是等待加入，无法添加 AI")
		return
	}

	// 加入 AI 玩家
	JoinMatchAsAI(r, genAIPlayerID(r))
}

type RemovePlayerPayload struct {
	PlayerID string `json:"playerID"`
}

func RemovePlayer(r *domain.Room, playerID string) bool {
	if _, ok := r.Players[playerID]; !ok {
		log.Printf("房间 %s 中不存在玩家 %s\n", r.ID, playerID)
		return false
	}

	delete(r.Players, playerID)
	for i, pid := range r.PlayerSeq {
		if pid == playerID {
			r.PlayerSeq = append(r.PlayerSeq[:i], r.PlayerSeq[i+1:]...)
			break
		}
	}
	log.Printf("玩家 %s 从房间 %s 移除\n", playerID, r.ID)
	return true
}

func HandleRemovePlayer(r *domain.Room, cmd domain.Command) {
	var p RemovePlayerPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}
	removePlayerID := p.PlayerID

	// 检查房间状态是否为等待加入
	if r.Status != dto.RoomStatusMatch {
		log.Println("❌ 房间状态不是等待加入，无法移除玩家")
		return
	}

	// 检查玩家是否存在
	if removePlayerConn, ok := r.Players[removePlayerID]; ok {
		// 给被移除的玩家发送错误消息
		msg := struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    "error",
			Message: "你已被移出房间",
		}

		data, err := json.Marshal(msg)
		if err == nil {
			removePlayerConn.Conn.WriteMessage(websocket.TextMessage, data)
			removePlayerConn.Conn.Close()
		}
	}

	// 移除玩家
	RemovePlayer(r, removePlayerID)
}

func HandleReadyMessage(r *domain.Room, cmd domain.Command) {
	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return
	}
	maxPlayers := roomInfo.MaxPlayers
	data.InitPlayerData(r, cmd.PlayerID)
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(r)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", r.ID, cmd.PlayerID, playerCount, maxPlayers)

	if playerCount == maxPlayers {
		err := data.SetRoomStatus(repository.Rdb, r.ID, true)
		if err != nil {
			log.Println("❌ 设置房间状态失败:", err)
			return
		}

		startKey := fmt.Sprintf("room:%s:game_start_time", r.ID)
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)

		playerID, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
		if err != nil {
			log.Println("❌ 获取当前玩家失败:", err)
			return
		}
		if playerID == "" {
			if len(r.Players) == 0 {
				log.Println("❌ 房间中没有玩家，无法设置当前玩家")
				return
			}

			playerIDs := make([]string, 0, len(r.Players))
			for pid := range r.Players {
				playerIDs = append(playerIDs, pid)
			}

			randomPlayerID := playerIDs[rand.Intn(len(playerIDs))]
			err = data.SetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID, randomPlayerID)
			if err != nil {
				log.Println("❌ 设置当前玩家失败:", err)
				return
			}
		}
		// 更新房间状态为匹配中
		err = SetRoomStatusCache(r.ID, dto.RoomStatusSetTile)
		if err != nil {
			log.Printf("❌ 内存设置房间状态失败: %v\n", err)
			return
		}
		err = data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusSetTile)
		if err != nil {
			log.Printf("❌ redis设置游戏状态失败: %v\n", err)
			return
		}
	}
}

// VirtualConn 虚拟连接，用于AI玩家
type VirtualConn struct {
	PlayerID string
	RoomID   string
}

// WriteMessage 实现ConnInterface接口
func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	// AI玩家不需要实际写入消息
	// log.Printf("[AI:%s] 收到消息到房间 %s: %s\n", v.PlayerID, v.RoomID, string(data))
	MaybeRunAIIfNeeded(v.RoomID, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, fmt.Errorf("virtual connection cannot read")
}

// Close 实现ConnInterface接口
func (v *VirtualConn) Close() error {
	// AI玩家不需要实际关闭连接
	return nil
}
