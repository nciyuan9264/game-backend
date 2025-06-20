package service

import (
	"encoding/json"
	"go-game/repository"
	"time"

	"math/rand"
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

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

func RandString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}
