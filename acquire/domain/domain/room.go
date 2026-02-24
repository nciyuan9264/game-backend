package domain

import (
	"encoding/json"
	"sync"
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

type Room struct {
	ID        string                 `json:"roomID"`
	OwnerID   string                 `json:"ownerID"`
	Status    RoomStatus             `json:"status"`
	Players   map[string]*PlayerConn `json:"players"`
	PlayerSeq []string               `json:"playerSeq"`
	CmdCh     chan Command           `json:"-"`
	QuitCh    chan struct{}          `json:"-"`

	DeleteTimer *time.Timer `json:"-"`
	RoomLock    sync.Mutex  `json:"-"`
}
