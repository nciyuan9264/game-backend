package roompkg

import (
	"log"
	"math/rand/v2"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// RoomService 是 splendor 房间的运行时服务（与 acquire 同型）。
type RoomService struct {
	Room *domain.Room
	svc  *roomcore.Service[*RoomService]
}

// MaxPlayers splendor 上限玩家数。
const MaxPlayers = 4

// Rooms 进程内所有房间索引。
var Rooms = roomcore.NewRegistry[*RoomService]()

// Run 是房间主循环：单 goroutine 串行处理命令，处理完成后广播。
func (r *RoomService) Run() {
	// 创建宽限期：60s 内无人连接 ws 则自动删除房间，兜底「创建后从未连接」的孤儿房。
	roomcore.StartCreateGrace(r.svc)
	for {
		select {
		case cmd := <-r.Room.CmdCh:
			r.handleCommand(cmd)
			roomcore.RearmThinkTimer(r.svc)
			if r.Room.State.RoomStatus == domain.RoomStatusMatch {
				game.BroadcastToMatch(r.Room)
			} else {
				game.BroadcastToRoom(r.Room)
			}
		case <-r.Room.QuitCh:
			roomcore.StopThinkTimer(r.svc)
			return
		}
	}
}

// handleCommand 处理房间命令
func (r *RoomService) handleCommand(cmd domain.Command) {
	switch cmd.Type {
	case "connect":
		roomcore.HandleConnect(r.svc, cmd)
	case "disconnect":
		roomcore.HandleDisconnect(r.svc, cmd)
	case "match_ready":
		roomcore.HandleMatchReady(r.svc, cmd)
	case "match_begin":
		HandleMatchBegin(r.Room, cmd)
	case "match_add_ai":
		roomcore.HandleAddAI(r.svc, cmd)
	case "match_remove_player":
		roomcore.HandleRemovePlayer(r.svc, cmd)
	case "game_ready":
		roomcore.HandleReadyMessage(r.svc, cmd)
	case "game_get_gem":
		game.HandleGetGemMessage(r.Room, cmd)
	case "game_buy_card":
		game.HandleBuyCardMessage(r.Room, cmd)
	case "game_preserve_card":
		game.HandleReserveCardMessage(r.Room, cmd)
	case "game_end":
		game.HandleGameEndMessage(r.Room, cmd)
	case "game_play_audio":
		game.HandlePlayAudioMessage(r.Room, cmd)
	case "game_restart_game":
		game.HandleRestartGameMessage(r.Room, cmd)
	default:
		log.Printf("⚠️ 未知命令类型: %s", cmd.Type)
	}
}

// HandleMatchBegin 检查是否所有玩家都准备好了，并初始化房间局内数据。
func HandleMatchBegin(r *domain.Room, cmd domain.Command) {
	allReady := true
	for _, pc := range r.Connections {
		if !pc.Ready {
			allReady = false
			break
		}
	}

	if !allReady {
		log.Printf("房间 %s 不是所有玩家都已准备", r.ID)
		return
	}

	r.State.RoomStatus = domain.RoomStatusWaiting
	r.State.MaxPlayers = len(r.Connections)

	// 初始化牌堆 / 宝石 / 贵族
	game.InitRoomData(r)

	// 初始化每个玩家的局内数据
	for playerID := range r.Connections {
		data.InitPlayerData(r, playerID)
		log.Printf("房间 %s 玩家 %s 进入游戏", r.ID, playerID)
	}

	r.State.RoomStatus = domain.RoomStatusWaiting

	log.Printf("房间 %s 所有玩家都已准备，开始游戏", r.ID)
}

// handleAllReady splendor 的全员到齐回调：rand.IntN 选第一个 + 写 FirstPlayer + 切到 Playing。
func handleAllReady(r *domain.Room, cmd domain.Command) {
	r.State.GameStartTime = time.Now()

	if r.State.CurrentPlayer == "" {
		if len(r.PlayerSeq) == 0 {
			log.Printf("房间 %s 中没有玩家，无法设置当前玩家", r.ID)
			return
		}

		firstPlayerID := r.PlayerSeq[rand.IntN(len(r.PlayerSeq))]
		r.State.CurrentPlayer = firstPlayerID
		r.State.FirstPlayer = firstPlayerID
	}

	if r.State.RoomStatus == domain.RoomStatusWaiting {
		log.Printf("房间 %s 所有玩家进入游戏，开始游戏", r.ID)
		r.State.RoomStatus = domain.RoomStatusPlaying
	}
}
