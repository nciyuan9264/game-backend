package domain

import (
	"encoding/json"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// 类型别名：保持外部使用 domain.Command 等不变
type WriteOnlyConn = roomcore.WriteOnlyConn
type ReadWriteConn = roomcore.ReadWriteConn
type Command = roomcore.Command
type PlayerConn = roomcore.PlayerConn

type Color int

const (
	ColorWhite Color = 0
	ColorBlack Color = 1
)

type CardNumber int

const (
	NumMinus1 CardNumber = -1
	Num0      CardNumber = 0
	Num1      CardNumber = 1
	Num2      CardNumber = 2
	Num3      CardNumber = 3
	Num4      CardNumber = 4
	Num5      CardNumber = 5
	Num6      CardNumber = 6
	Num7      CardNumber = 7
	Num8      CardNumber = 8
	Num9      CardNumber = 9
	Num10     CardNumber = 10
	Num11     CardNumber = 11
)

type Card struct {
	ID         string     `json:"id"`         // "1A"
	Color      Color      `json:"color"`      // 颜色
	Num        CardNumber `json:"num"`        // 数字
	IsRevealed bool       `json:"isRevealed"` // 是否被揭示
	Index      int        `json:"index"`      // 牌组中的索引
}

type PlayerState struct {
	Cards []*Card `json:"cards"`
}

const DefaultMaxPlayers = 2

type LastAction struct {
	Action   string          `json:"action"`
	PlayerID string          `json:"playerID"`
	Payload  json.RawMessage `json:"payload"`
}

type GameState struct {
	CurrentPlayer string     `json:"currentPlayer"`
	GameStartTime time.Time  `json:"gameStartTime"`
	RoomStatus    RoomStatus `json:"roomStatus"`
	OwnerID       string     `json:"ownerID"`
	MaxPlayers    int        `json:"maxPlayers"`

	BoardCards map[string]*Card        `json:"boardCards"`
	Players    map[string]*PlayerState `json:"players"`
	LastData   *LastAction             `json:"lastData"`
}

type Room struct {
	*roomcore.Base

	State *GameState `json:"state"`
}
