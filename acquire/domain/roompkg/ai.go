package roompkg

import (
	"encoding/json"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/rand"
)

func chooseTileForAI(roomID, playerID string) string {
	tiles, err := data.GetPlayerTiles(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil || len(tiles) == 0 {
		return ""
	}

	allTiles, err := data.GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取所有房间瓦片失败", utils.F("room_id", roomID), utils.F("error", err))
		return ""
	}

	// 遍历 AI 玩家拥有的 tiles
	for _, tileID := range tiles {
		neighbors := data.GetAdjacentTileKeys(tileID)
		for _, nID := range neighbors {
			if neighborTile, ok := allTiles[nID]; ok && neighborTile.Belong != "" {
				return tileID
			}
		}
	}

	return tiles[rand.Intn(len(tiles))]
}

func shouldCreateCompany(roomID, playerID string) bool {
	// 你可以根据 redis 状态判断：该 tile 旁边没有现有公司 && 有未创建公司
	return true // 示例：这里直接返回 true
}

func chooseCompanyForAI(roomID string) string {
	companyInfo, err := data.GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取公司信息失败", utils.F("room_id", roomID), utils.F("error", err))
		return ""
	}
	// 过滤掉已创建的公司
	var uncreated []string
	for company, info := range companyInfo {
		if info.Tiles == 0 {
			uncreated = append(uncreated, company)
		}
	}

	// 优先级分类
	priority1 := []string{"Continental", "Imperial"}
	priority2 := []string{"American", "Festival", "Worldwide"}
	var p1, p2, p3 []string

	for _, c := range uncreated {
		if utils.StringInSlice(c, priority1) {
			p1 = append(p1, c)
		} else if utils.StringInSlice(c, priority2) {
			p2 = append(p2, c)
		} else {
			p3 = append(p3, c)
		}
	}

	// 从高优先级到低依次尝试选择
	if len(p1) > 0 {
		return p1[rand.Intn(len(p1))]
	}
	if len(p2) > 0 {
		return p2[rand.Intn(len(p2))]
	}
	return p3[rand.Intn(len(p3))]
}

func chooseStocksToBuyForAI(roomID, playerID string) map[string]int {
	companyInfo, err := data.GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取公司信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}
	playerInfo, err := data.GetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, playerID, "money")
	if err != nil {
		utils.Error("获取玩家信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}
	money := playerInfo.Money

	playerStock, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		utils.Error("获取玩家股票失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}

	// 收集可购买的公司（已创建，且有库存，且价格不超过总金额）
	type candidate struct {
		Name   string
		Price  int
		Remain int
	}
	var options []candidate
	for name, info := range companyInfo {
		if info.Tiles > 0 && info.StockPrice <= money && info.StockTotal > 0 && playerStock[name] < 13 {
			options = append(options, candidate{
				Name:   name,
				Price:  info.StockPrice,
				Remain: info.StockTotal,
			})
		}
	}

	if len(options) == 0 {
		return map[string]int{}
	}

	// 从便宜到贵排序（贪婪）
	sort.Slice(options, func(i, j int) bool {
		return options[i].Price < options[j].Price
	})

	result := make(map[string]int)
	stockCount := 0
	for _, opt := range options {
		maxCanBuy := min(3-stockCount, opt.Remain, money/opt.Price)
		if maxCanBuy <= 0 {
			continue
		}

		result[opt.Name] = maxCanBuy
		money -= maxCanBuy * opt.Price
		stockCount += maxCanBuy

		if stockCount >= 3 || money <= 0 {
			break
		}
	}

	return result
}

func min(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func IsAIPlayer(playerID string) bool {
	return strings.HasPrefix(playerID, "ai_") // 简单策略，也可以是数据库字段
}

func chooseMergingSettleForAI(roomID, playerID string) []dto.MergingSettleItem {
	playerData, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		utils.Error("获取玩家股票信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}

	mergeSettleData, err := data.GetMergeSettleData(repository.Ctx, repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取合并数据失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}

	mainCompany, err := data.GetMergeMainCompany(repository.Rdb, repository.Ctx, roomID)
	if err != nil {
		utils.Error("获取合并主公司失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}

	companyInfo, err := data.GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取公司信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return nil
	}

	result := []dto.MergingSettleItem{}

	for companyKey := range mergeSettleData {
		count := playerData[companyKey]
		if count == 0 {
			continue
		}
		mainCompanyInfo := companyInfo[mainCompany]
		company := companyInfo[companyKey]

		exchangeAmount := 0
		sellAmount := count

		if company.StockPrice/2 >= mainCompanyInfo.StockPrice {
			// 可交换的最大偶数股数（不超过 count 且为偶数）
			maxEven := count
			if maxEven%2 != 0 {
				maxEven -= 1
			}

			// 主公司最多能接受的交换数（以 2 股换 1 股）
			maxCanExchange := mainCompanyInfo.StockTotal * 2

			// 取两者中较小的
			exchangeAmount = min2(maxEven, maxCanExchange)
			sellAmount = count - exchangeAmount
		}

		result = append(result, dto.MergingSettleItem{
			Company:        companyKey,
			SellAmount:     sellAmount,
			ExchangeAmount: exchangeAmount,
		})
	}

	return result
}

func chooseMergingSelectionForAI(roomID, playerID string, mainCompany []string) string {
	companyInfo, err := data.GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取公司信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return ""
	}

	playerStocks, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		utils.Error("获取玩家股票信息失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return ""
	}
	res := ""
	max := -1
	for _, companyKey := range mainCompany {
		stockInUse := 25 - companyInfo[companyKey].StockTotal
		if stockInUse == 0 {
			continue // 避免除以 0
		}
		num := playerStocks[companyKey] / stockInUse
		if num > max {
			max = num
			res = companyKey
		}
	}

	return res
}

func MaybeRunAIIfNeeded(roomID string, message []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		utils.Error("AI 消息格式错误", utils.F("room_id", roomID), utils.F("error", err))
		return false
	}

	// 提取当前玩家
	roomData, ok := msg["roomData"].(map[string]interface{})
	if !ok {
		return false
	}
	currentPlayerID, ok := roomData["currentPlayer"].(string)
	if !ok || currentPlayerID == "" {
		return false
	}

	// 提取当前状态
	roomInfo, ok := roomData["roomInfo"].(map[string]interface{})
	if !ok {
		return false
	}

	gameStatusStr, ok := roomInfo["gameStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := dto.RoomStatus(gameStatusStr)

	playerId, ok := msg["playerId"].(string)
	if !ok || playerId == "" || (playerId != currentPlayerID && gameStatus != dto.RoomStatusMergingSettle) {
		return false
	}
	// 判断是否是 AI 玩家 - 检查两种情况：
	// 1. 玩家ID以 "ai_" 开头（原始AI玩家）
	// 2. 或者在房间的玩家列表中，该玩家被标记为AI
	isAI := IsAIPlayer(currentPlayerID)
	if !isAI && gameStatus != dto.RoomStatusMergingSettle {
		// 检查房间内存中的玩家状态
		roomService, roomExists := Rooms[roomID]
		if roomExists {
			utils.Info("当前玩家 %s 尝试运行 AI", utils.F("player_id", currentPlayerID))
			player, playerExists := roomService.Room.Players[currentPlayerID]
			utils.Info("当前玩家 %s 存在于房间中", utils.F("player_id", currentPlayerID), utils.F("playerExists", playerExists))
			if playerExists && player.AI {
				isAI = true
				utils.Info("检测到被替换为AI的玩家", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID))
			}
		}

		utils.Info("当前玩家 %s 不是 AI 玩家", utils.F("player_id", currentPlayerID))

		if !isAI {
			return false
		}
	}

	// 提取临时数据（合并选择）
	tempData, ok := msg["tempData"].(map[string]interface{})
	if !ok {
		utils.Error("tempData 类型错误", utils.F("room_id", roomID))
		return false
	}

	var mainCompany []string
	if mergeSel, ok := tempData["merge_selection_temp"].(map[string]interface{}); ok {
		if raw, ok := mergeSel["mainCompany"]; ok {
			// 安全类型断言
			if arr, ok := raw.([]interface{}); ok {
				for _, item := range arr {
					if s, ok := item.(string); ok {
						mainCompany = append(mainCompany, s)
					}
				}
			}
		}
	}

	// mergingSettle 特殊校验
	if gameStatus == dto.RoomStatusMergingSettle {
		mergeSettleData, err := data.GetMergeSettleData(repository.Ctx, repository.Rdb, roomID)
		if err != nil {
			utils.Error("获取合并数据失败", utils.F("room_id", roomID), utils.F("player_id", playerId), utils.F("error", err))
			return false
		}

		// 仅当玩家在合并对象中时才进行 AI 操作
		playerInHoder := false
		for _, data := range mergeSettleData {
			if (len(data.Hoders)) == 0 {
				continue
			}
			if data.Hoders[0] == playerId {
				playerInHoder = true
				break
			}
		}
		if !playerInHoder {
			utils.Error("外层校验玩家不在任何合并中", utils.F("room_id", roomID), utils.F("player_id", playerId))
			return false
		}
	}

	allTile, err := data.GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		utils.Error("获取所有 tile 失败", utils.F("room_id", roomID), utils.F("player_id", playerId), utils.F("error", err))
		return false
	}
	isAllTileUsed := true
	for _, tile := range allTile {
		if tile.Belong == "" {
			isAllTileUsed = false
		}
	}
	if isAllTileUsed {
		utils.Error("所有 tile 已被使用", utils.F("room_id", roomID), utils.F("player_id", playerId))
		// time.Sleep(3 * time.Second)
		// SetGameStatus(repository.Rdb, roomID, dto.RoomStatusEnd)
	}

	utils.Info("当前是 AI 玩家的回合，准备延迟执行 AI 行动", utils.F("room_id", roomID), utils.F("player_id", playerId), utils.F("game_status", gameStatus))

	// ---------- 在协程中延迟执行 ----------
	go func() {
		time.Sleep(5 * time.Second)

		conn := &VirtualConn{PlayerID: currentPlayerID, RoomID: roomID}
		var aiMsg map[string]interface{}

		switch gameStatus {
		case "setTile":
			tile := chooseTileForAI(roomID, currentPlayerID)
			if tile == "" {
				utils.Error("AI 未选择有效 tile", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID))
				return
			}
			utils.Info("AI 选择 tile", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("tile", tile))
			aiMsg = map[string]interface{}{
				"type":    "game_place_tile",
				"payload": map[string]interface{}{"tileKey": tile},
			}
		case "createCompany":
			company := chooseCompanyForAI(roomID)
			if company == "" {
				utils.Error("AI 未选择有效公司", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID))
				return
			}
			utils.Info("AI 选择公司", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("company", company))
			aiMsg = map[string]interface{}{
				"type":    "game_create_company",
				"payload": map[string]interface{}{"company": company},
			}
		case "buyStock":
			stocks := chooseStocksToBuyForAI(roomID, currentPlayerID)
			utils.Info("AI 选择购买股票", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("stocks", stocks))
			aiMsg = map[string]interface{}{
				"type":    "game_buy_stock",
				"payload": map[string]interface{}{"stocks": stocks},
			}
		case "mergingSelection":
			selection := chooseMergingSelectionForAI(roomID, currentPlayerID, mainCompany)
			utils.Info("AI 选择合并公司", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("selection", selection))
			aiMsg = map[string]interface{}{
				"type":    "game_merging_selection",
				"payload": map[string]interface{}{"mainCompany": selection},
			}
		case "mergingSettle":
			settle := chooseMergingSettleForAI(roomID, playerId)
			utils.Info("AI 选择合并结算", utils.F("room_id", roomID), utils.F("player_id", playerId), utils.F("settle", settle))
			aiMsg = map[string]interface{}{
				"type":    "game_merging_settle",
				"payload": map[string]interface{}{"actions": settle},
			}
		case "end":
			utils.Info("AI 选择重新开始游戏", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID))
			aiMsg = map[string]interface{}{
				"type": "game_restart_game",
			}
		default:
			utils.Warn("当前状态未定义 AI 行为", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("game_status", gameStatus))
			return
		}

		// 加入 playerID 然后交给 handler 执行
		// 将 AI 消息转换为 Command 格式，和玩家一样通过通道传递
		payload, err := json.Marshal(aiMsg["payload"])
		if err != nil {
			utils.Error("AI 消息序列化失败", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("error", err))
			return
		}
		utils.Info("AI 发送消息", utils.F("room_id", roomID), utils.F("player_id", currentPlayerID), utils.F("message", string(payload)))

		// 向房间的命令通道发送消息，和玩家一样的处理方式
		Rooms[roomID].Room.CmdCh <- domain.Command{
			Type:     aiMsg["type"].(string),
			PlayerID: playerId,
			Payload:  payload,
			Conn:     conn,
		}

		utils.Info("AI 发送命令", utils.F("room_id", roomID), utils.F("player_id", playerId), utils.F("command_type", aiMsg["type"]))
	}()

	return true
}
