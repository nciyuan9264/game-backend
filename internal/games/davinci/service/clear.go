package service

import (
	"fmt"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

func ScheduleDailyRoomReset() {
	for {
		duration := durationUntilNext4AM()
		fmt.Printf("距离下次清空还有：%v\n", duration)

		time.Sleep(duration)

		// 清空 Rooms
		fmt.Println("⏰ 清空房间 Rooms")
		clearRooms()
	}
}

// 计算距离下一个“北京时间今天/明天 4:00”的时间
func durationUntilNext4AM() time.Duration {
	now := time.Now().In(timeutil.Beijing)

	today4 := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, timeutil.Beijing)

	next := today4
	if !now.Before(today4) {
		next = today4.Add(24 * time.Hour)
	}
	return time.Until(next)
}

func clearRooms() {
	for k := range roompkg.Rooms.Snapshot() {
		roompkg.Rooms.Delete(k)
	}
	fmt.Println("✅ Rooms 已清空")
}
