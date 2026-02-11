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
	"go-game/utils"
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

func startDelayedDelete(r *domain.Room) {
	// 防止重复启动定时器
	if r.DeleteTimer != nil {
		return
	}

	utils.Info("房间进入 10 秒延迟删除状态", utils.F("room_id", r.ID))

	r.DeleteTimer = time.AfterFunc(10*time.Second, func() {
		r.RoomLock.Lock()
		defer r.RoomLock.Unlock()

		// 再次确认：是否仍然没有在线真人
		for _, p := range r.Players {
			if !p.AI && p.Online {
				utils.Info("房间有玩家重连，取消删除", utils.F("room_id", r.ID))
				r.DeleteTimer = nil
				return
			}
		}
		delete(Rooms, r.ID)
		utils.Info("房间已被延迟删除", utils.F("room_id", r.ID))
	})
}

func transferOwnerOrDelete(r *domain.Room) bool {
	for _, p := range r.Players {
		if !p.AI && p.Online {
			r.OwnerID = p.PlayerID
			p.Ready = true
			return false
		}
	}

	// 没有在线真人了
	startDelayedDelete(r)
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
		utils.Info("玩家已从匹配队列中移除", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	} else {
		p, ok := r.Players[playerID]
		if !ok {
			return
		}

		// 关闭连接，停止心跳
		if p.Conn != nil {
			p.Conn.Close()
		}

		p.Online = false
		p.Conn = nil

		utils.Info("玩家标记为离线", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	}

	utils.Info("玩家是否是房主", utils.F("room_id", r.ID), utils.F("player_id", playerID), utils.F("is_owner", ownerLeft))
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
			utils.Error("获取房间信息失败", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
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
		utils.Warn("未知命令类型", utils.F("command_type", cmd.Type))
	}
}

// handleConnectCommand 处理玩家加入命令
func handleConnectCommand(r *domain.Room, cmd domain.Command) {
	playerID := cmd.PlayerID
	conn := cmd.Conn

	// 1️⃣ 已存在玩家（重连 or 重复）
	if p, ok := r.Players[playerID]; ok {
		utils.Info("玩家尝试加入房间", utils.F("room_id", r.ID), utils.F("player_id", playerID), utils.F("current_status", p.Online))
		if !p.Online {
			// 检查是否是真实连接
			if realConn, ok := conn.(*dto.RealConn); ok {
				realConn.StartHeartbeat()
			}
			p.Conn = conn
			p.Online = true
			utils.Info("玩家重连成功", utils.F("room_id", r.ID), utils.F("player_id", playerID))
			return
		}

		// 已在线，重复连接
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"PLAYER_ALREADY_IN_ROOM"}`))
		conn.Close()
		return
	}

	utils.Info("玩家尝试加入房间（新玩家）", utils.F("room_id", r.ID), utils.F("player_id", playerID))

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
	// 检查是否是真实连接
	if realConn, ok := conn.(*dto.RealConn); ok {
		realConn.StartHeartbeat()
	}

	r.Players[playerID] = &dto.PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
		Ready:    r.OwnerID == playerID,
		AI:       false,
	}
	r.PlayerSeq = append(r.PlayerSeq, playerID)

	utils.Info("玩家加入房间", utils.F("room_id", r.ID), utils.F("player_id", playerID))
}

type ReadyPayload struct {
	Ready bool `json:"ready"`
}

func HandleMatchReady(r *domain.Room, cmd domain.Command) {
	var p ReadyPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		utils.Error("无效的 payload", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
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
		utils.Error("不是所有玩家都已准备", utils.F("room_id", r.ID))
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
		utils.Error("初始化房间信息失败", utils.F("room_id", r.ID), utils.F("error", err))
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
			utils.Error("初始化公司失败", utils.F("room_id", r.ID), utils.F("company_id", id), utils.F("error", err))
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
				utils.Error("tile 序列化失败", utils.F("room_id", r.ID), utils.F("tile_id", id), utils.F("error", err))
				return
			}
			pipe.HSet(ctx, tileKey, id, tileJSON)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		utils.Error("tile 初始化 Redis 写入失败", utils.F("room_id", r.ID), utils.F("error", err))
		return
	}

	for playerID, player := range r.Players {
		if player.AI {
			data.InitPlayerData(r, playerID)
			utils.Info("AI 玩家加入房间", utils.F("room_id", r.ID), utils.F("player_id", playerID))
		}
	}

	// 更新房间状态为匹配中
	err = SetRoomStatusCache(r.ID, dto.RoomStatusWaiting)
	if err != nil {
		utils.Error("内存设置房间状态失败", utils.F("room_id", r.ID), utils.F("error", err))
		return
	}
	err = data.SetGameStatus(rdb, r.ID, dto.RoomStatusWaiting)
	if err != nil {
		utils.Error("redis设置游戏状态失败", utils.F("room_id", r.ID), utils.F("error", err))
		return
	}

	// 开始游戏
	utils.Info("所有玩家都已准备，开始游戏", utils.F("room_id", r.ID))
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

	utils.Info("AI 玩家加入房间", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	return true
}

func HandleAddAI(r *domain.Room, cmd domain.Command) {
	// 检查房间是否已满
	if len(r.Players) >= MaxPlayers {
		utils.Error("房间已满，无法添加 AI", utils.F("room_id", r.ID))
		return
	}

	// 检查房间状态是否为等待加入
	if r.Status != dto.RoomStatusMatch {
		utils.Error("房间状态不是等待加入，无法添加 AI", utils.F("room_id", r.ID), utils.F("current_status", r.Status))
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
		utils.Error("房间中不存在玩家", utils.F("room_id", r.ID), utils.F("player_id", playerID))
		return false
	}

	delete(r.Players, playerID)
	for i, pid := range r.PlayerSeq {
		if pid == playerID {
			r.PlayerSeq = append(r.PlayerSeq[:i], r.PlayerSeq[i+1:]...)
			break
		}
	}
	utils.Info("玩家从房间移除", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	return true
}

func HandleRemovePlayer(r *domain.Room, cmd domain.Command) {
	var p RemovePlayerPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		utils.Error("无效的 payload", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
		return
	}
	removePlayerID := p.PlayerID

	// 检查房间状态是否为等待加入
	if r.Status != dto.RoomStatusMatch {
		utils.Error("房间状态不是等待加入，无法移除玩家", utils.F("room_id", r.ID), utils.F("current_status", r.Status))
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
		utils.Error("无法获取房间信息", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
		return
	}
	maxPlayers := roomInfo.MaxPlayers
	data.InitPlayerData(r, cmd.PlayerID)
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(r)
	utils.Info("玩家加入", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("current_count", playerCount), utils.F("max_players", maxPlayers))

	if playerCount == maxPlayers {
		err := data.SetRoomStatus(repository.Rdb, r.ID, true)
		if err != nil {
			utils.Error("设置房间状态失败", utils.F("room_id", r.ID), utils.F("error", err))
			return
		}

		startKey := fmt.Sprintf("room:%s:game_start_time", r.ID)
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)

		playerID, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
		if err != nil {
			utils.Error("获取当前玩家失败", utils.F("room_id", r.ID), utils.F("error", err))
			return
		}
		if playerID == "" {
			if len(r.Players) == 0 {
				utils.Error("房间中没有玩家，无法设置当前玩家", utils.F("room_id", r.ID))
				return
			}

			playerIDs := make([]string, 0, len(r.Players))
			for pid := range r.Players {
				playerIDs = append(playerIDs, pid)
			}

			randomPlayerID := playerIDs[rand.Intn(len(playerIDs))]
			err = data.SetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID, randomPlayerID)
			if err != nil {
				utils.Error("设置当前玩家失败", utils.F("room_id", r.ID), utils.F("error", err))
				return
			}
		}
		// 更新房间状态为匹配中
		if roomInfo.GameStatus == dto.RoomStatusWaiting {
			err = SetRoomStatusCache(r.ID, dto.RoomStatusSetTile)
			if err != nil {
				utils.Error("内存设置房间状态失败", utils.F("room_id", r.ID), utils.F("error", err))
				return
			}
			err = data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusSetTile)
			if err != nil {
				utils.Error("redis设置游戏状态失败", utils.F("room_id", r.ID), utils.F("error", err))
				return
			}
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
