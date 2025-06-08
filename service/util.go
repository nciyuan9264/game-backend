package service

import (
	"encoding/json"
	"go-game/repository"
)

func JsonMarshal(v interface{}) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func IsRoomFull(roomID string) bool {
	count, err := repository.Rdb.SCard(repository.Ctx, "room:"+roomID+":players").Result()
	if err != nil {
		return false // 或者根据实际情况处理错误
	}
	return count >= 2
}
