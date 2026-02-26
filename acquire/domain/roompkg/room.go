package roompkg

import (
	"encoding/json"
	"fmt"
	"time"

	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/domain/game"
	"go-game/utils"
	"sync"

	"sort"

	"github.com/gorilla/websocket"
)

type RoomService struct {
	Room *domain.Room
}

func (r *RoomService) Run() {
	for {
		select {
		case cmd := <-r.Room.CmdCh:
			r.handleCommand(cmd)
			if r.Room.State.RoomStatus == domain.RoomStatusMatch {
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

func GetAllRoomsSnapshot() map[string]*RoomService {
	RoomLock.Lock()
	defer RoomLock.Unlock()

	copied := make(map[string]*RoomService)
	for k, v := range Rooms {
		copied[k] = v
	}
	return copied
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
	case "game_play_audio":
		game.HandlePlayAudioMessage(r.Room, cmd)
	case "game_restart_game":
		game.HandleRestartGameMessage(r.Room, cmd)
	default:
		utils.Warn("未知命令类型", utils.F("command_type", cmd.Type))
	}
}

func startDelayedDelete(r *domain.Room) {
	// 防止重复启动定时器
	if r.DeleteTimer != nil {
		return
	}
	utils.Info("房间10s后将被删除", utils.F("room_id", r.ID))
	r.DeleteTimer = time.AfterFunc(10*time.Second, func() {
		// 再次确认：是否仍然没有在线真人
		for _, p := range r.Connections {
			if !p.AI && p.Online {
				utils.Info("房间有玩家重连，取消删除", utils.F("room_id", r.ID))
				r.DeleteTimer = nil
				return
			}
		}
		// 通知房间主循环退出
		close(r.QuitCh)
		delete(Rooms, r.ID)
		utils.Info("房间已被延迟删除", utils.F("room_id", r.ID))
	})
}

func transferOwnerOrDelete(r *domain.Room) bool {
	for _, p := range r.Connections {
		if !p.AI && p.Online {
			r.State.OwnerID = p.PlayerID
			p.Ready = true
			utils.Info("房主已变更", utils.F("room_id", r.ID), utils.F("player_id", p.PlayerID))
			return false
		}
	}

	if r.State.RoomStatus == domain.RoomStatusMatch {
		// 没有在线真人了
		startDelayedDelete(r)
		return true
	}
	return false
}

func MarkPlayerOffline(r *domain.Room, playerID string) (roomDeleted bool) {
	if r.State.RoomStatus == domain.RoomStatusMatch {
		for i, pID := range r.PlayerSeq {
			if pID == playerID {
				r.PlayerSeq = append(r.PlayerSeq[:i], r.PlayerSeq[i+1:]...)
				break
			}
		}
		delete(r.Connections, playerID)
		utils.Info("玩家离开房间", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	} else {
		p, ok := r.Connections[playerID]
		if !ok {
			return
		}

		// 关闭连接，停止心跳
		if p.Conn != nil {
			p.Conn.Close()
		}

		p.Online = false
		p.Conn = nil

		// 设置3分钟定时器，检测是否需要替换为AI
		if !p.AI {
			// 取消之前的定时器（如果有）
			if p.OfflineTimer != nil {
				p.OfflineTimer.Stop()
			}

			utils.Info("玩家离开房间,开始ai替换计时", utils.F("room_id", r.ID), utils.F("player_id", playerID))
			p.OfflineTimer = time.AfterFunc(2*time.Minute, func() {

				// 再次检查玩家状态
				player, ok := r.Connections[playerID]
				if !ok || player.Online || player.AI {
					return
				}

				// 检查房间中是否还有其他在线的真实玩家
				hasOnlineRealPlayer := false
				for _, otherPlayer := range r.Connections {
					if !otherPlayer.AI && otherPlayer.Online {
						hasOnlineRealPlayer = true
						break
					}
				}

				if hasOnlineRealPlayer {
					// 替换为AI
					ReplacePlayerWithAI(r, playerID)
				}
			})
		}
	}

	if playerID == r.State.OwnerID {
		return transferOwnerOrDelete(r)
	}

	return false
}

// 玩家断开连接后，从房间中移除该连接
func handleDisconnectCommand(r *domain.Room, cmd domain.Command) {
	// 1️⃣ 通知 room：这个人掉线了
	MarkPlayerOffline(r, cmd.PlayerID)

	// 2️⃣ 同步 Redis 房间状态（如果房间还在）
	// if !roomDeleted && r.State.RoomStatus != domain.RoomStatusMatch {
	// roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	// data.SetRoomStatus(repository.Rdb, r.ID, false)
	// r.State.RoomStatus = domain.RoomStatusMatch
	// }
}

// 获取房间中玩家数量
func getRoomPlayerCount(room *domain.Room) int {
	onLineCount := 0
	for _, pc := range room.Connections {
		if pc.Online {
			onLineCount++
		}
	}
	return onLineCount
}

// handleConnectCommand 处理玩家加入命令
func handleConnectCommand(r *domain.Room, cmd domain.Command) {
	playerID := cmd.PlayerID
	conn := cmd.Conn

	// 1️⃣ 已存在玩家（重连 or 重复）
	if p, ok := r.Connections[playerID]; ok {
		utils.Info("玩家尝试加入房间（已存在玩家）", utils.F("room_id", r.ID), utils.F("player_id", playerID), utils.F("current_status", p.Online))
		if !p.Online || p.AI {
			// 检查是否是真实连接
			if realConn, ok := conn.(*domain.RealConn); ok {
				realConn.StartHeartbeat()
			}
			p.Conn = conn
			p.Online = true

			// 取消离线定时器（如果有）
			if p.OfflineTimer != nil {
				p.OfflineTimer.Stop()
				p.OfflineTimer = nil
			}

			// 如果玩家之前被替换为AI，恢复为真实玩家
			if p.AI {
				p.AI = false
				utils.Info("玩家从AI恢复为真实玩家", utils.F("room_id", r.ID), utils.F("player_id", playerID))
			}
			p.Conn = cmd.Conn
			p.Online = true

			utils.Info("玩家重连成功", utils.F("room_id", r.ID), utils.F("player_id", playerID))
			return
		} else {
			// 已在线，重复连接
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"玩家已在房间内,请从其他设备退出"}`))
			conn.Close()
			return
		}
	} else {
		// 2️⃣ 状态校验
		if r.State.RoomStatus != domain.RoomStatusMatch {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"游戏已开始，无法加入"}`))
			conn.Close()
			return
		}

		// 3️⃣ 人数校验
		if len(r.Connections) >= MaxPlayers {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
			conn.Close()
			return
		}

		// 检查是否是真实连接
		if realConn, ok := conn.(*domain.RealConn); ok {
			realConn.StartHeartbeat()
		}

		r.Connections[playerID] = &domain.PlayerConn{
			PlayerID: playerID,
			Conn:     conn,
			Online:   true,
			Ready:    r.State.OwnerID == playerID,
			AI:       false,
		}
		r.PlayerSeq = append(r.PlayerSeq, playerID)

		utils.Info("玩家加入房间", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	}
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
	for _, pc := range r.Connections {
		if pc.PlayerID == cmd.PlayerID {
			pc.Ready = ready
			break
		}
	}
}

func HandleMatchBegin(r *domain.Room, cmd domain.Command) {
	// 检查是否所有玩家都已准备
	allReady := true
	for _, pc := range r.Connections {
		if !pc.Ready {
			allReady = false
			break
		}
	}

	if !allReady {
		utils.Error("不是所有玩家都已准备", utils.F("room_id", r.ID))
		return
	}

	r.State.RoomStatus = domain.RoomStatusWaiting
	r.State.MaxPlayers = len(r.Connections)

	// 初始化公司数据
	r.State.Companies = map[string]*domain.CompanyState{
		"Sackson": {
			Name:       "Sackson",
			StockTotal: 25,
			Tiles:      0,   // 初始数量
			StockPrice: 200, // 初始参考股价（可调整）
		},
		"Tower": {
			Name:       "Tower",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"American": {
			Name:       "American",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Festival": {
			Name:       "Festival",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Worldwide": {
			Name:       "Worldwide",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Continental": {
			Name:       "Continental",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Imperial": {
			Name:       "Imperial",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
	}

	// 初始化游戏棋盘（12x9 个 tile）
	for col := 1; col <= 12; col++ {
		for row := 'A'; row <= 'I'; row++ {
			id := fmt.Sprintf("%d%c", col, row)
			tile := &domain.Tile{
				ID:     id,
				Belong: "",
			}
			r.State.BoardTiles[id] = tile
		}
	}

	for playerID := range r.Connections {
		data.InitPlayerData(r, playerID)
		utils.Info("玩家进入游戏", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	}

	// 更新房间状态为匹配中
	r.State.RoomStatus = domain.RoomStatusWaiting

	// 开始游戏
	utils.Info("所有玩家都已准备，开始游戏", utils.F("room_id", r.ID))
}

func JoinMatchAsAI(r *domain.Room, playerID string) bool {
	r.Connections[playerID] = &domain.PlayerConn{
		PlayerID: playerID,
		Conn:     &VirtualConn{Room: r},
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
	if len(r.Connections) >= MaxPlayers {
		utils.Error("房间已满，无法添加 AI", utils.F("room_id", r.ID))
		return
	}

	// 检查房间状态是否为等待加入
	if r.State.RoomStatus != domain.RoomStatusMatch {
		utils.Error("房间状态不是等待加入，无法添加 AI", utils.F("room_id", r.ID), utils.F("current_status", r.State.RoomStatus))
		return
	}

	// 加入 AI 玩家
	JoinMatchAsAI(r, genAIPlayerID(r))
}

type RemovePlayerPayload struct {
	PlayerID string `json:"playerID"`
}

func RemovePlayer(r *domain.Room, playerID string) bool {
	if _, ok := r.Connections[playerID]; !ok {
		utils.Error("房间中不存在玩家", utils.F("room_id", r.ID), utils.F("player_id", playerID))
		return false
	}

	delete(r.Connections, playerID)
	for i, pid := range r.PlayerSeq {
		if pid == playerID {
			r.PlayerSeq = append(r.PlayerSeq[:i], r.PlayerSeq[i+1:]...)
			break
		}
	}
	utils.Info("玩家从房间移除", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	return true
}

// ReplacePlayerWithAI 将离线玩家替换为AI玩家
func ReplacePlayerWithAI(r *domain.Room, playerID string) {
	player, ok := r.Connections[playerID]
	if !ok {
		utils.Error("房间中不存在玩家", utils.F("room_id", r.ID), utils.F("player_id", playerID))
		return
	}

	// 取消定时器
	if player.OfflineTimer != nil {
		player.OfflineTimer.Stop()
		player.OfflineTimer = nil
	}

	// 标记玩家为AI
	player.AI = true
	player.Online = true // AI玩家始终在线
	player.Conn = &VirtualConn{Room: r}
	game.BroadcastToRoom(r)
	utils.Info("玩家替换为AI", utils.F("room_id", r.ID), utils.F("player_id", playerID))
	utils.Info("Redis中玩家状态已更新为AI", utils.F("room_id", r.ID), utils.F("player_id", playerID))
}

func HandleRemovePlayer(r *domain.Room, cmd domain.Command) {
	var p RemovePlayerPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		utils.Error("无效的 payload", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
		return
	}
	removePlayerID := p.PlayerID

	// 检查房间状态是否为等待加入
	if r.State.RoomStatus != domain.RoomStatusMatch {
		utils.Error("房间状态不是等待加入，无法移除玩家", utils.F("room_id", r.ID), utils.F("current_status", r.State.RoomStatus))
		return
	}

	// 检查玩家是否存在
	if removePlayerConn, ok := r.Connections[removePlayerID]; ok {
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
	maxPlayers := r.State.MaxPlayers
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(r)

	if playerCount == maxPlayers {
		r.State.GameStartTime = time.Now()

		if r.State.CurrentPlayer == "" {
			if len(r.Connections) == 0 {
				utils.Error("房间中没有玩家，无法设置当前玩家", utils.F("room_id", r.ID))
				return
			}

			playerIDs := make([]string, 0, len(r.Connections))
			for pid, pc := range r.Connections {
				if pc != nil {
					playerIDs = append(playerIDs, pid)
				}
			}
			if len(playerIDs) == 0 {
				utils.Error("没有在线玩家，无法设置当前玩家", utils.F("room_id", r.ID))
				return
			}
			sort.Strings(playerIDs)
			firstPlayerID := playerIDs[0]
			r.State.CurrentPlayer = firstPlayerID
		}
		// 更新房间状态为匹配中
		if r.State.RoomStatus == domain.RoomStatusWaiting {
			utils.Info("所有玩家进入游戏，开始游戏", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID))
			r.State.RoomStatus = domain.RoomStatusSetTile
		}
	}
}
