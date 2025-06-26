package ws

import (
	"encoding/json"
	"fmt"
	"go-game/repository"
)

type LastAction struct {
	Action   string          `json:"action"` // get_gem / buy_card / reserve_card
	PlayerID string          `json:"playerID"`
	Payload  json.RawMessage `json:"payload"` // 原始 JSON 数据，延迟反序列化
}

// SetLastTileKey 保存刚才放置的tile
func SetLastData(roomID, playerID string, action string, payload interface{}) error {
	lastDataKey := fmt.Sprintf("room:%s:last_data", roomID)

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 Payload 失败: %w", err)
	}

	data := LastAction{
		Action:   action,
		PlayerID: playerID,
		Payload:  raw,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化 LastAction 失败: %w", err)
	}

	field := fmt.Sprintf("player:%s", playerID)
	return repository.Rdb.HSet(repository.Ctx, lastDataKey, field, bytes).Err()
}

func GetLastData(roomID, playerID string) (*LastAction, error) {
	lastDataKey := fmt.Sprintf("room:%s:last_data", roomID)
	field := fmt.Sprintf("player:%s", playerID)

	val, err := repository.Rdb.HGet(repository.Ctx, lastDataKey, field).Result()
	if err != nil {
		return nil, err
	}

	var action LastAction
	if err := json.Unmarshal([]byte(val), &action); err != nil {
		return nil, fmt.Errorf("反序列化 LastAction 失败: %w", err)
	}

	return &action, nil
}
