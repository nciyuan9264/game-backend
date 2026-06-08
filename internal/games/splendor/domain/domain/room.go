package domain

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// RealConn 是带心跳的真实 WebSocket 连接包装。
type RealConn struct {
	*websocket.Conn
	PingInterval time.Duration
	LastPongTime time.Time
	Done         chan struct{}
	closeOnce    sync.Once
}

func (r *RealConn) WriteMessage(messageType int, data []byte) error {
	return r.Conn.WriteMessage(messageType, data)
}

func (r *RealConn) Close() error {
	r.closeOnce.Do(func() {
		close(r.Done)
		r.Conn.Close()
	})
	return nil
}

func (r *RealConn) StartHeartbeat() {
	go func() {
		ticker := time.NewTicker(r.PingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if time.Since(r.LastPongTime) > r.PingInterval*2 {
					r.Close()
					return
				}
				r.WriteMessage(websocket.PingMessage, []byte{})
				log.Println("✅ 发送心跳包")
			case <-r.Done:
				return
			}
		}
	}()

	r.Conn.SetPongHandler(func(string) error {
		r.LastPongTime = time.Now()
		return nil
	})
}

func NewRealConn(conn *websocket.Conn) *RealConn {
	return &RealConn{
		Conn:         conn,
		PingInterval: 30 * time.Second,
		LastPongTime: time.Now(),
		Done:         make(chan struct{}),
	}
}

// 类型别名：保持外部使用 domain.Command 等不变
type WriteOnlyConn = roomcore.WriteOnlyConn
type ReadWriteConn = roomcore.ReadWriteConn
type Command = roomcore.Command
type PlayerConn = roomcore.PlayerConn

// RoomStatus 是房间内部的字符串枚举。
type RoomStatus string

const (
	RoomStatusMatch    RoomStatus = "match"     // 大厅 / 匹配阶段
	RoomStatusWaiting  RoomStatus = "waiting"   // 等待进入游戏
	RoomStatusPlaying  RoomStatus = "playing"   // 进行中
	RoomStatusLastTurn RoomStatus = "last_turn" // 最后一轮
	RoomStatusEnd      RoomStatus = "end"       // 已结束
)

// PlayerState 玩家在 splendor 局内的全部状态。
type PlayerState struct {
	NormalCard  []entities.NormalCard `json:"normalCard"`
	NobleCard   []entities.NobleCard  `json:"nobleCard"`
	Gem         map[string]int        `json:"gem"`
	Score       int                   `json:"score"`
	ReserveCard []entities.NormalCard `json:"reserveCard"`
}

// LastAction 最近一次操作记录，用于同步给前端。
type LastAction struct {
	Action   string          `json:"action"`
	PlayerID string          `json:"playerID"`
	Payload  json.RawMessage `json:"payload"`
}

// GameState splendor 房间局内全部状态。
type GameState struct {
	CurrentPlayer string     `json:"currentPlayer"`
	FirstPlayer   string     `json:"firstPlayer"`
	GameStartTime time.Time  `json:"gameStartTime"`
	RoomStatus    RoomStatus `json:"roomStatus"`
	OwnerID       string     `json:"ownerID"`
	MaxPlayers    int        `json:"maxPlayers"`

	// 牌堆 / 宝石 / 贵族
	NormalCards map[string]*entities.NormalCard `json:"normalCards"` // key = strconv.Itoa(card.ID)
	NobleCards  map[string]*entities.NobleCard  `json:"nobleCards"`
	Gems        map[string]int                  `json:"gems"`

	// 玩家局内数据
	Players map[string]*PlayerState `json:"players"`

	// 最近一次操作
	LastData *LastAction `json:"lastData"`
}

// Room splendor 房间结构（内嵌 *roomcore.Base）。
type Room struct {
	*roomcore.Base

	State *GameState `json:"state"`
}
