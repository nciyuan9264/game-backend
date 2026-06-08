package service

import (
	"fmt"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/repository"
	"time"
)

func ScheduleDailyRoomReset() {
	for {
		duration := durationUntilNext4AM()
		fmt.Printf("距离下次清空还有：%v\n", duration)

		time.Sleep(duration)

		fmt.Println("⏰ 清空房间 Rooms")
		clearRooms()
	}
}

// 计算距离下一个“今天/明天 4:00”的时间
func durationUntilNext4AM() time.Duration {
	now := time.Now()

	// 今天 4:00
	today4 := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		4, 0, 0, 0,
		now.Location(),
	)

	var next time.Time

	if now.Before(today4) {
		// 还没到今天4点
		next = today4
	} else {
		// 已经过了今天4点 → 明天4点
		next = today4.Add(24 * time.Hour)
	}

	return time.Until(next)
}

func clearRooms() {
	for k := range roompkg.Rooms.Snapshot() {
		roompkg.Rooms.Delete(k)
	}

	err := repository.Rdb.FlushDB(repository.Ctx).Err()
	if err != nil {
		fmt.Println("清空 Redis 失败:", err)
	} else {
		fmt.Println("✅ Redis 已清空")
	}
}
