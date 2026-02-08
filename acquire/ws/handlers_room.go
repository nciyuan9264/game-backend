package ws

import (
	"go-game/domain/room"
)

var roomHandlers = map[string]GameHandler{
	"match_ready":         room.HandlePlayerReady,
	"match_begin":         HandleMatchBegin,
	"match_add_ai":        HandleAddAI,
	"match_remove_player": HandleRemovePlayer,
}

const MaxPlayers = 6
