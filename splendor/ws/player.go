package ws

import (
	"fmt"
	"go-game/repository"
	"log"
)

// 初始化玩家数据
func InitPlayerData(roomID string, playerID string) error {
	// 检查玩家数据是否已存在
	exists, err := IsPlayerInfoExists(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println(err)
		return err
	}
	if exists {
		return fmt.Errorf("玩家数据已存在")
	}
	// 初始化玩家数据
	err = InitPlayerDataToRedis(roomID, playerID)
	if err != nil {
		log.Println("设置玩家信息失败:", err)
	}

	return nil
}
