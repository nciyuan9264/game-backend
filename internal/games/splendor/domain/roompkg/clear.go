package roompkg

import (
	"fmt"
	"time"
)

// ScheduleWeeklyRoomReset 每周一 4 点清空内存中的房间表（不再触碰 Redis）。
func ScheduleWeeklyRoomReset() {
	for {
		duration := durationUntilNextMonday4AM()
		fmt.Printf("距离下次清空还有：%v\n", duration)

		time.Sleep(duration)

		fmt.Println("⏰ 清空房间 Rooms")
		clearRooms()
	}
}

func durationUntilNextMonday4AM() time.Duration {
	now := time.Now()
	daysUntilMonday := int(time.Monday - now.Weekday())
	if daysUntilMonday <= 0 {
		daysUntilMonday += 7
	}

	next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 4, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(7 * 24 * time.Hour)
	}
	return next.Sub(now)
}

func clearRooms() {
	for k := range Rooms.Snapshot() {
		Rooms.Delete(k)
	}
}
