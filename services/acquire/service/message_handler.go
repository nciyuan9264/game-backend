package service

import (
	"acquire-service/ws"
	"context"
	"fmt"
)

type MessageHandler struct {
	gameService *GameService
}

func NewMessageHandler(gameService *GameService) *MessageHandler {
	return &MessageHandler{
		gameService: gameService,
	}
}

func (h *MessageHandler) HandleMessage(ctx context.Context, roomID, playerID string, msg ws.Message) (interface{}, error) {
	// msg.Data 现在包含完整的msgMap，包括playerID和payload
	msgMap := msg.Data
	
	// 根据消息类型调用相应的游戏服务方法
	switch msg.Type {
	case "place_tile":
		return h.gameService.HandlePlaceTile(ctx, roomID, playerID, msgMap["payload"])
	case "buy_stock":
		return h.gameService.HandleBuyStock(ctx, roomID, playerID, msgMap["payload"])
	case "create_company":
		return h.gameService.HandleCreateCompany(ctx, roomID, playerID, msg.Data)
	case "ready":
		return h.gameService.HandleReady(ctx, roomID, playerID, msg.Data)
	case "merging_settle":
		return h.gameService.HandleMergingSettle(ctx, roomID, playerID, msg.Data)
	case "merging_selection":
		return h.gameService.HandleMergingSelection(ctx, roomID, playerID, msg.Data)
	case "game_end":
		return h.gameService.HandleGameEnd(ctx, roomID, playerID, msg.Data)
	case "restart_game":
		return h.gameService.HandleRestartGame(ctx, roomID, playerID, msg.Data)
	default:
		return nil, fmt.Errorf("未知消息类型: %s", msg.Type)
	}
}
