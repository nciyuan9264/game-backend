package ws

import (
	"go-game/domain/game"
	"go-game/domain/room"
	"go-game/dto"

	"github.com/go-redis/redis/v8"
)

type GameHandler func(conn room.ReadWriteConn, rdb *redis.Client, currentRoom *dto.Room, playerID string, msgMap map[string]interface{})

var GameHandlers = map[string]GameHandler{
	"place_tile":        game.HandlePlaceTileMessage,
	"create_company":    game.HandleCreateCompanyMessage,
	"merging_settle":    game.HandleMergingSettleMessage,
	"buy_stock":         game.HandleBuyStockMessage,
	"merging_selection": game.HandleMergingSelectionMessage,
	"game_end":          game.HandleGameEndMessage,
	"play_audio":        game.HandlePlayAudioMessage,
	"restart_game":      game.HandleRestartGameMessage,
	"game_ready":        room.HandleReadyMessage,
}
