package service

import (
	"acquire-service/dto"
	"acquire-service/repository"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/rand"
)

type VirtualConn struct {
	PlayerID string
	RoomID   string
}

func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	// log.Printf("[AI:%s] 发送消息到房间 %s: %s\n", v.PlayerID, v.RoomID, string(data))
	MaybeRunAIIfNeeded(v.RoomID, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, fmt.Errorf("virtual connection cannot read")
}

func (v *VirtualConn) Close() error {
	return nil
}

var _ WriteOnlyConn = (*VirtualConn)(nil) // 编译期断言实现

func chooseTileForAI(roomID, playerID string) string {
	tiles, err := GetPlayerTiles(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil || len(tiles) == 0 {
		return ""
	}

	allTiles, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取所有房间瓦片失败:", err)
		return ""
	}

	// 遍历 AI 玩家拥有的 tiles
	for _, tileID := range tiles {
		neighbors := getAdjacentTileKeys(tileID)
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
	companyInfo, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
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
		if StringInSlice(c, priority1) {
			p1 = append(p1, c)
		} else if StringInSlice(c, priority2) {
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

func chooseStocksToBuyForAI(roomID, playerID string) map[string]interface{} {
	companyInfo, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return nil
	}
	playerInfo, err := GetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, playerID, "money")
	if err != nil {
		log.Println("❌ 获取玩家信息失败:", err)
		return nil
	}
	money := playerInfo.Money

	playerStock, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家股票失败:", err)
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
		return map[string]interface{}{}
	}

	// 从便宜到贵排序（贪婪）
	sort.Slice(options, func(i, j int) bool {
		return options[i].Price < options[j].Price
	})

	result := make(map[string]interface{})
	stockCount := 0
	for _, opt := range options {
		maxCanBuy := min(3-stockCount, opt.Remain, money/opt.Price)
		if maxCanBuy <= 0 {
			continue
		}

		result[opt.Name] = float64(maxCanBuy)
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
	playerData, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家股票信息失败:", err)
		return nil
	}

	mergeSettleData, err := GetMergeSettleData(repository.Ctx, repository.Rdb, roomID)
	if err != nil {
		log.Printf("❌ 获取合并数据失败: %v\n", err)
		return nil
	}

	mainCompany, err := GetMergeMainCompany(repository.Rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取合并主公司失败:", err)
		return nil
	}

	companyInfo, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
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
	companyInfo, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return ""
	}

	playerStocks, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家股票信息失败:", err)
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

func MaybeRunAIIfNeeded(roomID string, data []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Println("❌ AI 消息格式错误:", err)
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

	// 判断是否是 AI 玩家
	if !IsAIPlayer(currentPlayerID) && gameStatus != dto.RoomStatusMergingSettle {
		return false
	}

	// 提取临时数据（合并选择）
	tempData, ok := msg["tempData"].(map[string]interface{})
	if !ok {
		log.Println("❌ tempData 类型错误")
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
		mergeSettleData, err := GetMergeSettleData(repository.Ctx, repository.Rdb, roomID)
		if err != nil {
			log.Printf("❌ 获取合并数据失败: %v\n", err)
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
			log.Println("❌ 外层校验玩家不在任何合并中")
			return false
		}
	}

	allTile, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取所有 tile 失败:", err)
		return false
	}
	isAllTileUsed := true
	for _, tile := range allTile {
		if tile.Belong == "" {
			isAllTileUsed = false
		}
	}
	if isAllTileUsed {
		log.Println("❌ 所有 tile 已被使用")
		// time.Sleep(3 * time.Second)
		// SetGameStatus(repository.Rdb, roomID, dto.RoomStatusEnd)
	}

	log.Printf("🤖 当前是 AI 玩家 %s 的回合，状态为 %s，准备延迟执行 AI 行动...", playerId, gameStatus)

	// ---------- 在协程中延迟执行 ----------
	// 在MaybeRunAIIfNeeded函数中修改第446行附近的代码
	go func() {
		time.Sleep(5 * time.Second)

		ctx := context.Background()
		gameService := &GameService{rdb: repository.Rdb}

		var err error

		switch gameStatus {
		case "setTile":
			tile := chooseTileForAI(roomID, currentPlayerID)
			if tile == "" {
				log.Println("🤖 AI 未选择有效 tile")
				return
			}
			_, err = gameService.HandlePlaceTile(ctx, roomID, currentPlayerID, map[string]interface{}{
				"type":    "place_tile",
				"payload": tile,
			})
		case "createCompany":
			company := chooseCompanyForAI(roomID)
			if company == "" {
				log.Println("🤖 AI 未选择有效公司")
				return
			}
			_, err = gameService.HandleCreateCompany(ctx, roomID, currentPlayerID, map[string]interface{}{
				"type":    "create_company",
				"payload": company,
			})
		case "buyStock":
			stocks := chooseStocksToBuyForAI(roomID, currentPlayerID)
			_, err = gameService.HandleBuyStock(ctx, roomID, currentPlayerID, map[string]interface{}{
				"type":    "buy_stock",
				"payload": stocks,
			})
		case "mergingSelection":
			selection := chooseMergingSelectionForAI(roomID, currentPlayerID, mainCompany)
			_, err = gameService.HandleMergingSelection(ctx, roomID, currentPlayerID, map[string]interface{}{
				"type":    "merging_selection",
				"payload": selection,
			})
		case "mergingSettle":
			settle := chooseMergingSettleForAI(roomID, playerId)
			_, err = gameService.HandleMergingSettle(ctx, roomID, playerId, map[string]interface{}{
				"type":    "merging_settle",
				"payload": settle,
			})
		case "end":
			_, err = gameService.HandleRestartGame(ctx, roomID, currentPlayerID, map[string]interface{}{
				"type": "restart_game",
			})
		default:
			log.Printf("⚠️ 当前状态 %s 未定义 AI 行为", gameStatus)
			return
		}

		if err != nil {
			log.Printf("❌ AI 操作失败: %v", err)
			return
		}

		log.Printf("🤖 AI [%s] 执行操作成功", playerId)

		// 通过Hub广播更新
		// 这里需要获取Hub实例并调用BroadcastToRoom
		// 或者保持使用旧的BroadcastToRoom函数
		BroadcastToRoom(roomID)
	}()

	return true
}

func JoinRoomAsAI(roomID, playerID string) bool {
	roomLock.Lock()
	defer roomLock.Unlock()

	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return false
	}

	maxPlayers := roomInfo.MaxPlayers

	// 判断房间人数是否已满
	if len(Rooms[roomID]) >= maxPlayers {
		log.Printf("房间 %s 已满，AI %s 无法加入\n", roomID, playerID)
		return false
	}

	InitPlayerData(roomID, playerID)
	// 加入房间，虚拟连接
	Rooms[roomID] = append(Rooms[roomID], dto.PlayerConn{
		PlayerID: playerID,
		Conn:     &VirtualConn{PlayerID: playerID, RoomID: roomID},
		Online:   true,
	})

	log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}
