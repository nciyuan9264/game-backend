// Package history 负责 acquire 对局历史的持久化与查询。
//
// 三张表：
//   - games        一行=一局
//   - game_players 一行=一局中一个玩家终局快照
//   - game_events  一行=一条命令（事件溯源，用于按步快进）
package history

import (
	"time"

	"gorm.io/datatypes"
)

// Game 一行 = 一局。
type Game struct {
	ID              int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	RoomID          string         `gorm:"size:64;index" json:"roomID"`
	GameType        string         `gorm:"size:32;index:idx_game_type_started,priority:1" json:"gameType"`
	StartedAt       time.Time      `gorm:"index:idx_game_type_started,priority:2,sort:desc;not null" json:"startedAt"`
	EndedAt         *time.Time     `json:"endedAt,omitempty"`
	DurationSeconds int            `json:"durationSeconds"`
	WinnerUserID    *int64         `json:"winnerUserID,omitempty"`
	WinnerPlayerID  string         `gorm:"size:64" json:"winnerPlayerID,omitempty"`
	MaxPlayers      int            `json:"maxPlayers"`
	InitialState    datatypes.JSON `json:"initialState"`
	FinalResult     datatypes.JSON `json:"finalResult,omitempty"` // 终局每个玩家的 money/stocks/total
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`

	Players []GamePlayer `gorm:"foreignKey:GameID;constraint:OnDelete:CASCADE" json:"players,omitempty"`
	Events  []GameEvent  `gorm:"foreignKey:GameID;constraint:OnDelete:CASCADE" json:"events,omitempty"`
}

func (Game) TableName() string { return "games" }

// GamePlayer 一行 = 一局中一个玩家终局快照。
type GamePlayer struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID      int64     `gorm:"index:idx_gp_game;not null" json:"gameID"`
	UserID      *int64    `gorm:"index:idx_gp_user_game,priority:1" json:"userID,omitempty"`
	PlayerID    string    `gorm:"size:64;not null" json:"playerID"`
	SeatIndex   int       `json:"seatIndex"`
	IsAI        bool      `json:"isAI"`
	FinalScore  *int      `json:"finalScore,omitempty"`
	FinalMoney  *int      `json:"finalMoney,omitempty"`
	FinalStocks int       `json:"finalStocks"`
	IsWinner    bool      `json:"isWinner"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (GamePlayer) TableName() string { return "game_players" }

// GameEvent 一行 = 一条命令。
type GameEvent struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID     int64          `gorm:"uniqueIndex:idx_ge_game_seq,priority:1;not null" json:"gameID"`
	Seq        int            `gorm:"uniqueIndex:idx_ge_game_seq,priority:2;not null" json:"seq"`
	OccurredAt time.Time      `json:"occurredAt"`
	PlayerID   string         `gorm:"size:64" json:"playerID"`
	CmdType    string         `gorm:"size:64;index" json:"cmdType"`
	Payload    datatypes.JSON `json:"payload"`
}

func (GameEvent) TableName() string { return "game_events" }
