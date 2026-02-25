package game

// func HandleGameEndMessage(r *domain.Room, cmd domain.Command) {
// 	err := data.SetGameStatus(repository.Rdb, r.ID, domain.RoomStatusEnd)
// 	if err != nil {
// 		utils.Error("设置游戏状态失败", utils.F("room_id", r.ID), utils.F("error", err))
// 		return
// 	}
// 	logPath := getGameLogFilePath(r.ID)
// 	utils.Info("游戏日志保存于", utils.F("log_path", logPath))
// }
