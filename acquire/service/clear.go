package service

import (
	"fmt"
	"go-game/domain/roompkg"
	"go-game/repository"
	"time"
)

func ScheduleWeeklyRoomReset() {
	for {
		duration := durationUntilNextMonday4AM()
		fmt.Printf("距离下次清空还有：%v\n", duration)

		time.Sleep(duration)

		// 清空 Rooms
		fmt.Println("⏰ 清空房间 Rooms")
		clearRooms()
	}
}

func durationUntilNextMonday4AM() time.Duration {
	now := time.Now()
	// 计算距离下周一的天数差
	daysUntilMonday := int(time.Monday - now.Weekday())
	if daysUntilMonday <= 0 {
		daysUntilMonday += 7
	}

	// 设置为下周一的4点
	next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 4, 0, 0, 0, now.Location())

	// 检查是否已经过了本周一的4点，如果是则设置为下周的4点
	if now.After(next) {
		next = next.Add(7 * 24 * time.Hour)
	}
	return next.Sub(now)
}

func clearRooms() {
	// 清空 map 的方式
	for k := range roompkg.Rooms {
		delete(roompkg.Rooms, k)
	}
	err := repository.Rdb.FlushDB(repository.Ctx).Err() // 清空当前数据库
	if err != nil {
		fmt.Println("清空 Redis 失败:", err)
	} else {
		fmt.Println("✅ Redis 已清空")
	}
}
