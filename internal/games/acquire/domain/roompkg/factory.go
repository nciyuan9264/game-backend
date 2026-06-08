package roompkg

import (
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// turnTimeoutByStatus acquire 各状态的思考时长。
// 未列出的状态返回 ok=false 表示不计时。
//
// 注：MergingSettle 是多玩家并发结算阶段，本计划范围内仍只跟随 CurrentPlayer 计时，
// 到点替 CurrentPlayer 拍一手；其他持股玩家的并发结算超时不做。
var turnTimeoutByStatus = map[domain.RoomStatus]time.Duration{
	domain.RoomStatusSetTile:          30 * time.Second,
	domain.RoomStatusCreateCompany:    20 * time.Second,
	domain.RoomStatusBuyStock:         40 * time.Second,
	domain.RoomStatusMergingSelection: 20 * time.Second,
	domain.RoomStatusMergingSettle:    60 * time.Second,
}

func NewRoomService(roomID, ownerID string) *RoomService {
	base := roomcore.NewBase(roomID, 128)
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			GameStartTime: time.Time{},
			LastTileKey:   "",
			RoomStatus:    domain.RoomStatusMatch,
			OwnerID:       ownerID,
			MaxPlayers:    MaxPlayers,
			CurrentPlayer: "",

			BoardTiles: make(map[string]*domain.Tile),
			Players:    make(map[string]*domain.PlayerState),
			Companies:  make(map[string]*domain.CompanyState),
		},
	}

	rs := &RoomService{Room: r}
	rs.svc = &roomcore.Service[*RoomService]{
		Base: base,
		Game: rs,

		GetMaxPlayers: func(rs *RoomService) int { return rs.Room.State.MaxPlayers },
		StatusOf:      func(rs *RoomService) string { return string(rs.Room.State.RoomStatus) },
		GetOwner:      func(rs *RoomService) string { return rs.Room.State.OwnerID },
		SetOwner: func(rs *RoomService, pid string) {
			rs.Room.State.OwnerID = pid
			if pc, ok := rs.Room.Connections[pid]; ok {
				pc.Ready = true
			}
		},
		OnAllReady: func(rs *RoomService, cmd roomcore.Command) {
			handleAllReady(rs.Room, cmd)
		},
		NewVirtualConn: func(rs *RoomService) roomcore.WriteOnlyConn {
			return &VirtualConn{Room: rs.Room}
		},
		OnAttachReader: func(rs *RoomService, conn roomcore.ReadWriteConn) {
			if rc, ok := conn.(*domain.RealConn); ok {
				rc.StartHeartbeat()
			}
		},
		OnRoomDeleted: func(rs *RoomService) { Rooms.Delete(rs.Room.ID) },
		GenAIPlayerID: func(rs *RoomService) string { return genAIPlayerID(rs.Room) },

		GetCurrentPlayer: func(rs *RoomService) string { return rs.Room.State.CurrentPlayer },
		GetTurnTimeout: func(rs *RoomService) (time.Duration, bool) {
			d, ok := turnTimeoutByStatus[rs.Room.State.RoomStatus]
			return d, ok
		},
		BuildTimeoutCommand: func(rs *RoomService, pid string) (roomcore.Command, bool) {
			return BuildTurnTimeoutCommand(rs.Room, pid)
		},

		Logger: coreLogger{},
	}
	return rs
}
