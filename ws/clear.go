package ws

import (
	"fmt"
	"time"
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

func durationUntilNext4AM() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())

	// 如果当前时间已过4点，则设置为第二天的4点
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func clearRooms() {
	// 清空 map 的方式
	// for k := range Rooms {
	// 	delete(Rooms, k)
	// }
	// err := repository.Rdb.FlushDB(repository.Ctx).Err() // 清空当前数据库
	// if err != nil {
	// 	fmt.Println("清空 Redis 失败:", err)
	// } else {
	// 	fmt.Println("✅ Redis 已清空")
	// }
}
