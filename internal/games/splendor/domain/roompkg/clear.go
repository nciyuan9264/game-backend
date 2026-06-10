package roompkg

import (
	"fmt"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

// ScheduleDailyRoomReset 每天北京时间 4 点清空内存中的房间表（不再触碰 Redis）。
func ScheduleDailyRoomReset() {
	for {
		duration := durationUntilNext4AM()
		fmt.Printf("距离下次清空还有：%v\n", duration)

		time.Sleep(duration)

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
	for k := range Rooms.Snapshot() {
		Rooms.Delete(k)
	}
}
