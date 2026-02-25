package domain

import (
	"encoding/json"
	"time"
)

// 持续监听客户端消息，并将其广播给房间内其他玩家
type WriteOnlyConn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// 读写接口，供真实客户端连接用，支持读取消息
type ReadWriteConn interface {
	WriteOnlyConn
	ReadMessage() (messageType int, p []byte, err error)
}

type Command struct {
	Type     string          `json:"type"`
	PlayerID string          `json:"playerID"`
	Payload  json.RawMessage `json:"payload"`
	Conn     ReadWriteConn   `json:"-"`
}

type PlayerConn struct {
	PlayerID     string        `json:"playerID"`
	Conn         WriteOnlyConn `json:"-"`
	Online       bool          `json:"online"`
	Ready        bool          `json:"ready"`
	AI           bool          `json:"ai"`
	OfflineTimer *time.Timer   `json:"-"`
}

type Tile struct {
	ID     string `json:"id"`     // "1A"
	Belong string `json:"belong"` // 公司名
}

type PlayerState struct {
	Money  int            `json:"money"`
	Stocks map[string]int `json:"stocks"`
	Tiles  []string       `json:"tiles"`
}

type MergingSelection struct {
	MainCompany  []string `json:"mainCompany"`
	OtherCompany []string `json:"otherCompany"`
}
type CompanyState struct {
	Name       string `json:"name"`
	Tiles      int    `json:"tiles"`
	StockTotal int    `json:"stockTotal"`
	StockPrice int    `json:"stockPrice"`
}
type GameState struct {
	CurrentPlayer string     `json:"currentPlayer"`
	GameStartTime time.Time  `json:"gameStartTime"`
	LastTileKey   string     `json:"lastTileKey"`
	RoomStatus    RoomStatus `json:"roomStatus"`
	OwnerID       string     `json:"ownerID"`
	MaxPlayers    int        `json:"maxPlayers"`

	BoardTiles map[string]*Tile         `json:"boardTiles"`
	Players    map[string]*PlayerState  `json:"players"`
	Companies  map[string]*CompanyState `json:"companies"`

	MergeMainCompany string                `json:"mergeMainCompany"`
	MergingSelection MergingSelection      `json:"mergingSelection"`
	MergeSettleData  map[string]SettleData `json:"mergeSettleData"`
}

type Room struct {
	ID string `json:"roomID"`

	State       *GameState             `json:"state"`
	Connections map[string]*PlayerConn `json:"connections"`
	PlayerSeq   []string               `json:"playerSeq"`

	CmdCh  chan Command  `json:"-"`
	QuitCh chan struct{} `json:"-"`

	DeleteTimer *time.Timer `json:"-"`
}
