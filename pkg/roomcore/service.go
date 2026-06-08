package roomcore

import "time"

// Logger 是 roomcore 的日志接口；各 game 提供适配器（zap / log.Printf 均可）。
// kv 形如 key1, val1, key2, val2 ...，length 应为偶数。
type Logger interface {
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Service 把 Base、game-specific RoomService(R) 与回调装在一起，作为生命周期函数的唯一入参。
// R 通常是各 game 自身的 *RoomService。
type Service[R any] struct {
	Base *Base
	Game R

	// GetMaxPlayers 返回当前房间允许的最大玩家数。
	GetMaxPlayers func(R) int
	// StatusOf 返回 game 自身的 RoomStatus 转字符串；只用于和 StatusMatch 比较。
	StatusOf func(R) string
	// GetOwner 读取当前房主的 PlayerID。
	GetOwner func(R) string
	// SetOwner 把 OwnerID 写到 game state，并把对应玩家 Ready 置 true。
	SetOwner func(R, string)
	// OnAllReady 当 HandleReadyMessage 检测到全员到齐时回调。
	// game 在此设置 CurrentPlayer / FirstPlayer / GameStartTime / RoomStatus 切换。
	OnAllReady func(R, Command)
	// NewVirtualConn 在 JoinMatchAsAI 中产生 AI 连接。
	NewVirtualConn func(R) WriteOnlyConn
	// OnAttachReader 在玩家加入 / 重连时被调用，用于让 game 启动心跳等。
	OnAttachReader func(R, ReadWriteConn)
	// OnRoomDeleted 房间被 startDelayedDelete 真正删除时回调。
	OnRoomDeleted func(R)
	// GenAIPlayerID HandleAddAI 中产生新的 AI 玩家 ID。
	GenAIPlayerID func(R) string

	// GetCurrentPlayer 返回当前轮到出手的玩家 ID（用于思考超时机制）。
	GetCurrentPlayer func(R) string
	// GetTurnTimeout 根据当前 RoomStatus 返回思考时长；返回 ok=false 表示当前状态不计时。
	GetTurnTimeout func(R) (time.Duration, bool)
	// BuildTimeoutCommand 由 game 决定思考超时时投递的具体命令。
	// 返回 ok=false 表示不投递任何命令（如 splendor AI 决策为空时）。
	BuildTimeoutCommand func(R, string) (Command, bool)

	Logger Logger
}
