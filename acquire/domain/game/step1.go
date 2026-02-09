package game

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"go-game/utils"
	"log"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

// PlaceTile 用于处理将棋子放置到棋盘上：修改 tile 的 belong 字段并更新 Redis，同时从玩家手牌中移除该 tile。
func placeTile(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	// Step 1：下棋
	if err := data.UpdateTileValue(rdb, roomID, tileKey, dto.Tile{ID: tileKey, Belong: "Blank"}); err != nil {
		return fmt.Errorf("❌ 写入 tile 出错: %w", err)
	}

	// Step 2：从玩家 tile 列表中移除该 tile
	if err := data.RemovePlayerTile(rdb, ctx, roomID, playerID, tileKey); err != nil {
		return err
	}

	// Step 3: 保存刚刚放置的 tileKey
	if err := data.SetLastTileKey(rdb, ctx, roomID, playerID, tileKey); err != nil {
		return err
	}

	log.Printf("✅ 玩家 %s 放置棋子 %s 成功\n", playerID, tileKey)
	return nil
}

func handleMergeProcess(
	rdb *redis.Client,
	room *domain.Room,
	mainHotel string,
	otherHotel []string,
	hotelTileCount map[string]int,
) error {
	tempSettleData := make(map[string]dto.SettleData)
	for _, hotel := range otherHotel {
		// Step 1：获取被并购酒店的 tile 数量
		tileCount, ok := hotelTileCount[hotel]
		if !ok {
			return fmt.Errorf("❌ 未找到酒店[%s]的 tile 数量", otherHotel)
		}
		// Step 2：遍历房间玩家，获取每人该公司股票数量
		type holder struct {
			PlayerID string
			Count    int
		}
		var holders []holder

		for _, pc := range room.Players {
			playerID := pc.PlayerID
			// 获取该玩家所有股票
			stockMap, err := data.GetPlayerStocks(rdb, repository.Ctx, room.ID, playerID)
			if err != nil {
				log.Printf("❌ 获取玩家[%s]股票失败: %v\n", playerID, err)
				continue
			}
			// 获取该玩家对该并购酒店的股票数
			count := stockMap[hotel] // 直接按 hotel 名称取股票数
			if count > 0 {
				holders = append(holders, holder{
					PlayerID: playerID,
					Count:    count,
				})
			}
		}
		// Step 3：根据持股数量排序
		sort.Slice(holders, func(i, j int) bool {
			return holders[i].Count > holders[j].Count
		})

		// 保存拥有当前公司股票的所有玩家
		currentCompanyHoders := make([]string, 0)
		for _, holder := range holders {
			currentCompanyHoders = append(currentCompanyHoders, holder.PlayerID)
		}

		stockInfo := utils.GetStockInfo(mainHotel, tileCount)
		firstBonus := stockInfo.BonusFirst
		secondBonus := stockInfo.BonusSecond

		dividends := make(map[string]int)

		// Step 4：分红逻辑
		firstGroup := []string{holders[0].PlayerID}
		firstCount := holders[0].Count

		// 找出第一名的并列玩家
		for i := 1; i < len(holders); i++ {
			if holders[i].Count == firstCount {
				firstGroup = append(firstGroup, holders[i].PlayerID)
			} else {
				break
			}
		}

		if len(firstGroup) > 1 {
			// 并列第一名：平分总红利（first + second）
			totalBonus := firstBonus + secondBonus
			each := totalBonus / len(firstGroup)
			for _, pid := range firstGroup {
				dividends[pid] = each
			}
		} else {
			// 第一名独占
			dividends[firstGroup[0]] = firstBonus

			// 找第二名（可能并列）
			secondGroup := []string{}
			secondCount := -1
			for i := 1; i < len(holders); i++ {
				if holders[i].Count < firstCount {
					if secondCount == -1 {
						secondCount = holders[i].Count
					}
					if holders[i].Count == secondCount {
						secondGroup = append(secondGroup, holders[i].PlayerID)
					} else {
						break
					}
				}
			}

			if len(secondGroup) > 0 {
				each := secondBonus / len(secondGroup)
				for _, pid := range secondGroup {
					dividends[pid] = each
				}
			}
		}

		for playerID, money := range dividends {
			if err := data.AddPlayerMoney(rdb, repository.Ctx, room.ID, playerID, money); err != nil {
				log.Printf("❌ 累加玩家[%s]红利失败: %v\n", playerID, err)
				return err
			}
		}
		tempSettleData[hotel] = dto.SettleData{
			Hoders:    currentCompanyHoders,
			Dividends: dividends,
		}
	}
	// 保存主公司到redis
	err := data.SetMergeMainCompany(rdb, repository.Ctx, room.ID, mainHotel)
	if err != nil {
		return err
	}
	err = data.SetMergeSettleData(repository.Ctx, rdb, room.ID, tempSettleData)
	if err != nil {
		return fmt.Errorf("❌ 保存结算数据失败: %w", err)
	}
	// Step 6：设置状态为“并购清算”
	err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusMergingSettle)
	if err != nil {
		log.Println("❌ 设置房间状态失败:", err)
	}
	log.Printf("✅ 完成酒店[%s]并入[%s]的红利计算和状态更新\n", otherHotel, mainHotel)
	return nil
}

func HandlePostTilePlacement(rdb *redis.Client, ctx context.Context, room *domain.Room, playerID string) error {
	// 第一步：获取公司信息
	companyInfo, err := data.GetCompanyInfo(rdb, room.ID)
	if err != nil {
		return fmt.Errorf("获取公司信息失败: %w", err)
	}

	// 第二步：检查是否有任何公司可购买股票
	for _, info := range companyInfo {
		if tilesCount := info.Tiles; tilesCount > 0 {
			// 有公司可买，设置房间状态为“买股票”
			if err := data.SetGameStatus(rdb, room.ID, dto.RoomStatusBuyStock); err != nil {
				return fmt.Errorf("更新房间状态失败: %w", err)
			}
			return nil
		}
	}
	// 发一张 tile
	if err := GiveRandomTileToPlayer(rdb, repository.Ctx, room, playerID); err != nil {
		return fmt.Errorf("发牌失败: %w", err)
	}

	// 切换玩家
	if err := SwitchToNextPlayer(rdb, repository.Ctx, room, playerID); err != nil {
		log.Println("切换玩家失败:", err)
	}
	return nil
}

func handleMergingLogic(rdb *redis.Client, room *domain.Room, playerID string, hotelSet map[string]struct{}) error {
	// 统计每个酒店的 tile 数量
	companyInfo, err := data.GetCompanyInfo(rdb, room.ID)
	if err != nil {
		return fmt.Errorf("获取公司信息失败: %w", err)
	}
	hotelTileCount := make(map[string]int)
	maxCount := 0
	for hotel := range hotelSet {
		info := companyInfo[hotel]
		tileCount := info.Tiles
		hotelTileCount[hotel] = tileCount
		if tileCount > maxCount {
			maxCount = tileCount
		}
	}
	// 找出最大 tile 数量的酒店
	var topHotels []string
	for hotel, count := range hotelTileCount {
		if count == maxCount {
			topHotels = append(topHotels, hotel)
		}
	}

	if len(topHotels) > 1 {
		for _, hotel := range topHotels {
			delete(hotelSet, hotel)
		}
		otherHotel := make([]string, 0, len(hotelSet))
		for key := range hotelSet {
			if companyInfo[key].Tiles >= 11 {
				continue
			}
			otherHotel = append(otherHotel, key)
		}
		if len(otherHotel) == 0 && maxCount >= 11 {
			err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有其他可以合并的公司")
			return nil
		}
		err = data.SetMergingSelection(rdb, repository.Ctx, room.ID, entities.MergingSelection{
			MainCompany:  topHotels,
			OtherCompany: otherHotel,
		})
		if err != nil {
			return err
		}
		err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusMergingSelection)
		if err != nil {
			return err
		}
		return nil
	} else {
		// 只有一个最大的酒店
		mainHotel := topHotels[0]
		delete(hotelSet, mainHotel)
		var otherHotel []string
		for key := range hotelSet {
			if companyInfo[key].Tiles >= 11 {
				continue
			}
			otherHotel = append(otherHotel, key)
		}
		if len(otherHotel) == 0 {
			err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有其他可以合并的公司")
			return nil
		}
		err = handleMergeProcess(rdb, room, mainHotel, otherHotel, hotelTileCount)
		if err != nil {
			return err
		}
	}
	return nil
}

// 检查是否有创建、并购、扩建规则触发
func checkTileTriggerRules(rdb *redis.Client, room *domain.Room, playerID string, tileKey string) error {
	adjTiles := data.GetAdjacentTileKeys(tileKey)
	companySet := make(map[string]struct{})
	blankTileCount := 0

	for _, adjKey := range adjTiles {
		tile, err := data.GetTileFromRedis(rdb, repository.Ctx, room.ID, adjKey)
		if err != nil {
			return fmt.Errorf("获取 tile 出错: %w", err)
		}

		switch tile.Belong {
		case "Blank":
			blankTileCount++
		case "": // 未被占用
			continue
		default:
			companySet[tile.Belong] = struct{}{}
		}
	}

	if len(companySet) >= 2 {
		log.Println("⚠️ 触发并购规则！邻接多个酒店:", companySet)
		err := handleMergingLogic(rdb, room, playerID, companySet)
		if err != nil {
			return err
		}
		return nil
	}

	if len(companySet) == 1 {
		log.Println("⚠️ 触发扩建公司规则！加入一个酒店:", companySet)
		var hotelList []string
		for key := range companySet {
			hotelList = append(hotelList, key)
		}
		company := hotelList[0]

		connectedTiles := data.GetConnectedTiles(rdb, room.ID, tileKey)
		for _, tileKeyBlank := range connectedTiles {
			// 写回 Redis
			if err := data.UpdateTileValue(rdb, room.ID, tileKeyBlank, dto.Tile{ID: tileKeyBlank, Belong: company}); err != nil {
				log.Printf("❌ 更新 tile %s 失败: %v", tileKeyBlank, err)
			} else {
				log.Printf("✅ 成功更新 tile %s 的归属为 %s", tileKeyBlank, company)
			}
		}

		companyKey := fmt.Sprintf("room:%s:company:%s", room.ID, company)
		// 获取公司 Hash 数据
		companyMap, err := rdb.HGetAll(repository.Ctx, companyKey).Result()
		if err != nil {
			return fmt.Errorf("获取公司数据失败: %w", err)
		}
		var companyData dto.Company
		decoderConfig := &mapstructure.DecoderConfig{
			DecodeHook: stringToIntHookFunc(),
			Result:     &companyData,
			TagName:    "json",
		}
		decoder, _ := mapstructure.NewDecoder(decoderConfig)
		if err := decoder.Decode(companyMap); err != nil {
			return fmt.Errorf("公司数据解析失败: %w", err)
		}
		// 统计公司 tiles 数量
		connectedTiles = data.GetConnectedTiles(rdb, room.ID, tileKey)
		companyData.Tiles = len(connectedTiles)

		// 写回 Hash
		companyUpdateMap := map[string]interface{}{
			"tiles": companyData.Tiles,
		}
		if err := rdb.HSet(repository.Ctx, companyKey, companyUpdateMap).Err(); err != nil {
			return fmt.Errorf("写回公司数据失败: %w", err)
		}
		log.Println("✅ 公司数据已更新:", companyData)

		err = HandlePostTilePlacement(repository.Rdb, repository.Ctx, room, playerID)
		if err != nil {
			log.Println("处理玩家放置 tile 后逻辑失败:", err)
		}
		return nil
	}

	if blankTileCount >= 1 {
		companyInfo, err := data.GetCompanyInfo(rdb, room.ID)
		if err != nil {
			return fmt.Errorf("获取公司信息失败: %w", err)
		}
		flag := false
		for _, info := range companyInfo {
			if info.Tiles == 0 {
				flag = true
				break
			}
		}
		if !flag {
			err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有可以创建的公司")
			return nil
		}

		log.Println("⚠️ 触发创建公司规则！创建一个酒店:")
		// Step 1: 修改房间状态为“创建公司状态”
		err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusCreateCompany)
		if err != nil {
			log.Println("❌ 设置房间状态失败:", err)
		}
		return nil
	}

	err := HandlePostTilePlacement(repository.Rdb, repository.Ctx, room, playerID)
	if err != nil {
		log.Println("处理玩家放置 tile 后逻辑失败:", err)
	}
	return nil
}

type PlaceTilePayload struct {
	TileKey string `json:"tileKey"`
}

// 处理玩家放置 tile 消息
func HandlePlaceTileMessage(r *domain.Room, cmd domain.Command) {
	var p PlaceTilePayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}
	tileKey := p.TileKey

	currentPlayer, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != cmd.PlayerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusSetTile {
		log.Println("❌ 不是放置 tile 的状态")
		return
	}

	// Step1: 放置棋子
	err = placeTile(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, tileKey)
	if err != nil {
		log.Println("放置棋子失败", tileKey)
		return
	}
	// Step2: 检查 创建公司/并购公司
	err = checkTileTriggerRules(repository.Rdb, r, cmd.PlayerID, tileKey)
	if err != nil {
		log.Println(err)
		return
	}
}

type MergingSelectionPayload struct {
	MainCompany string `json:"mainCompany"`
}

func HandleMergingSelectionMessage(r *domain.Room, cmd domain.Command) {

	var p MergingSelectionPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}
	maincompany := p.MainCompany

	currentPlayer, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != cmd.PlayerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusMergingSelection {
		log.Println("❌ 不是 merging_selection 的状态")
		return
	}

	mergeSelectionTemp, err := data.GetMergingSelection(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		log.Println("❌ 获取合并选择失败:", err)
		return
	}
	companyInfo, err := data.GetCompanyInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return
	}

	for _, company := range mergeSelectionTemp.MainCompany {
		if company == maincompany {
			continue
		}
		mergeSelectionTemp.OtherCompany = append(mergeSelectionTemp.OtherCompany, company)
	}

	hotelTileCount := make(map[string]int)
	maxCount := 0
	for i := len(mergeSelectionTemp.OtherCompany) - 1; i >= 0; i-- {
		hotel := mergeSelectionTemp.OtherCompany[i]
		info := companyInfo[hotel]
		if info.Tiles >= 11 {
			mergeSelectionTemp.OtherCompany = utils.RemoveAtIndex(mergeSelectionTemp.OtherCompany, i)
			continue
		}
		tileCount := info.Tiles
		hotelTileCount[hotel] = tileCount
		if tileCount > maxCount {
			maxCount = tileCount
		}
	}

	err = handleMergeProcess(repository.Rdb, r, maincompany, mergeSelectionTemp.OtherCompany, hotelTileCount)
	if err != nil {
		log.Println("❌ 处理合并过程失败:", err)
		return
	}
}

type MergingSettlePayload struct {
	Actions []dto.MergingSettleItem `json:"actions"`
}

func HandleMergingSettleMessage(r *domain.Room, cmd domain.Command) {

	var p MergingSettlePayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}

	settleActions := p.Actions

	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusMergingSettle {
		log.Println("❌ 不是合并 的状态")
		return
	}

	mergeSettleData, err := data.GetMergeSettleData(repository.Ctx, repository.Rdb, r.ID)
	if err != nil {
		log.Printf("❌ 获取合并数据失败: %v\n", err)
		return
	}

	playerInHoder := false
	for _, data := range mergeSettleData {
		oldHoders := data.Hoders
		for _, h := range oldHoders {
			if h == cmd.PlayerID {
				playerInHoder = true
			}
		}
	}
	if !playerInHoder {
		log.Println("❌ 玩家不在任何合并中")
		return
	}
	lockKey := fmt.Sprintf("lock:merge_settle:%s", r.ID)
	lockValue := uuid.NewString()
	locked, err := repository.Rdb.SetNX(repository.Ctx, lockKey, lockValue, 5*time.Second).Result()
	if err != nil || !locked {
		log.Printf("⚠️ 玩家[%s]尝试结算但加锁失败，可能有人在操作中...\n", cmd.PlayerID)
		return
	}
	defer func() {
		val, err := repository.Rdb.Get(repository.Ctx, lockKey).Result()
		if err == nil && val == lockValue {
			repository.Rdb.Del(repository.Ctx, lockKey)
		}
	}()

	// payloadRaw := msgMap["payload"]

	// // 将 interface{} 编码成 JSON
	// payloadBytes, err := json.Marshal(payloadRaw)
	// if err != nil {
	// 	log.Println("❌ payload 编码失败:", err)
	// 	return
	// }

	// // 反序列化为结构体切片
	// var settleActions []dto.MergingSettleItem
	// if err := json.Unmarshal(payloadBytes, &settleActions); err != nil {
	// 	log.Println("❌ payload 反序列化失败:", err)
	// 	return
	// }

	companyInfo, err := data.GetCompanyInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return
	}

	stockMap, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID)
	if err != nil {
		log.Printf("❌ 获取玩家[%s]股票失败: %v\n", cmd.PlayerID, err)
		return
	}

	mergeMainCompany, err := data.GetMergeMainCompany(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		log.Printf("❌ 获取合并主公司失败: %v\n", err)
		return
	}

	for _, item := range settleActions {
		companyData, ok := companyInfo[item.Company]
		if !ok {
			log.Printf("❌ 找不到公司[%s]的信息\n", item.Company)
			continue
		}

		sellAmount := int(item.SellAmount)
		exchangeAmount := int(item.ExchangeAmount)

		if sellAmount > 0 {
			stockMap[item.Company] -= sellAmount
			money := sellAmount * companyData.StockPrice
			if err := data.AddPlayerMoney(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, money); err != nil {
				log.Printf("❌ 扣除玩家[%s]股票失败: %v\n", cmd.PlayerID, err)
				return
			}
		}

		if exchangeAmount > 0 {
			// 修改股票持仓
			stockMap[mergeMainCompany] += exchangeAmount / 2
			stockMap[item.Company] -= exchangeAmount
		}
	}

	err = data.SetPlayerStocks(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, stockMap)
	if err != nil {
		log.Printf("❌ 保存玩家[%s]股票失败: %v\n", cmd.PlayerID, err)
		return
	}

	allHodersCleared := true
	// 移除 Hoders 中的 playerID
	for key, data := range mergeSettleData {
		oldHoders := data.Hoders
		newHoders := make([]string, 0)
		for _, h := range oldHoders {
			if h != cmd.PlayerID {
				newHoders = append(newHoders, h)
			}
		}
		data.Hoders = newHoders
		mergeSettleData[key] = data
		// 如果还有剩余 hoders，就不是全部清空
		if len(newHoders) > 0 {
			allHodersCleared = false
		}
	}

	if allHodersCleared {
		lastTile, err := data.GetLastTileKey(repository.Rdb, repository.Ctx, r.ID)
		if err != nil {
			log.Printf("❌ 获取当前创建公司 tile key 失败: %v\n", err)
			return
		}

		connTile := data.GetConnectedTiles(repository.Rdb, r.ID, lastTile)
		connTileSet := make(map[string]struct{})
		for _, id := range connTile {
			connTileSet[id] = struct{}{}
		}

		tileMap, err := data.GetAllRoomTiles(repository.Rdb, r.ID)
		if err != nil {
			log.Printf("❌ 获取房间 tile 信息失败: %v\n", err)
			return
		}

		for key, tile := range tileMap {
			if _, ok := mergeSettleData[tile.Belong]; ok {
				tile.Belong = mergeMainCompany
				tileMap[key] = tile
			}
			if tile.ID == lastTile {
				tile.Belong = mergeMainCompany
				tileMap[key] = tile
			}
			if _, ok := connTileSet[tile.ID]; ok {
				tile.Belong = mergeMainCompany
				tileMap[key] = tile
			}
		}

		err = data.SetAllRoomTiles(repository.Rdb, r.ID, tileMap)
		if err != nil {
			log.Printf("❌ 保存房间 tile 信息失败: %v\n", err)
			return
		}
		if err != nil {
			log.Printf("❌ 获取最后一个 tile key 失败: %v\n", err)
			return
		}
		adj := data.GetAdjacentTileKeys(lastTile)
		for _, key := range adj {
			tile, err := data.GetTileFromRedis(repository.Rdb, repository.Ctx, r.ID, key)
			if err != nil {
				log.Printf("❌ 获取 tileBelong 失败: %v\n", err)
				return
			}
			if tile.Belong == "Blank" {
				tile.Belong = mergeMainCompany
				err = data.UpdateTileValue(repository.Rdb, r.ID, key, tile)
				if err != nil {
					log.Printf("❌ 更新 tileBelong 失败: %v\n", err)
					return
				}
			}
		}

		err = data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusBuyStock)
		if err != nil {
			log.Printf("❌ 设置游戏状态失败: %v\n", err)
			return
		}
		if err := data.SetMergeSettleData(repository.Ctx, repository.Rdb, r.ID, map[string]dto.SettleData{}); err != nil {
			log.Printf("❌ 保存结算数据失败: %v\n", err)
			return
		}
	} else {
		// 保存结果
		if err := data.SetMergeSettleData(repository.Ctx, repository.Rdb, r.ID, mergeSettleData); err != nil {
			log.Printf("❌ 保存结算数据失败: %v\n", err)
			return
		}
	}
}

type CreateCompanyPayload struct {
	Company string `json:"company"`
}

func HandleCreateCompanyMessage(r *domain.Room, cmd domain.Command) {
	var p CreateCompanyPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("无效的 payload:", err)
		return
	}
	company := p.Company
	log.Println("✅ 收到 create_company 消息，目标 company:", company)

	currentPlayer, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != cmd.PlayerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusCreateCompany {
		log.Println("❌ 不是创建公司的状态")
		return
	}

	// Step 1: 取出 createTileKey
	createTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", r.ID)
	tileKey, err := repository.Rdb.Get(repository.Ctx, createTileKey).Result()
	if err != nil {
		log.Println("❌ 获取 createTileKey 失败:", err)
		return
	}
	log.Println("✅ 创建公司使用的 tileKey:", tileKey)

	// Step 2: 修改公司数据（仍用 Hash 类型保存）
	companyKey := fmt.Sprintf("room:%s:company:%s", r.ID, company)

	// 获取公司 Hash 数据
	companyMap, err := repository.Rdb.HGetAll(repository.Ctx, companyKey).Result()
	if err != nil {
		log.Println("❌ 获取公司 Hash 数据失败:", err)
		return
	}
	if len(companyMap) == 0 {
		log.Println("❌ 公司 Hash 数据为空")
		return
	}

	var companyData dto.Company
	decoderConfig := &mapstructure.DecoderConfig{
		DecodeHook: stringToIntHookFunc(),
		Result:     &companyData,
		TagName:    "json",
	}
	decoder, _ := mapstructure.NewDecoder(decoderConfig)
	if err := decoder.Decode(companyMap); err != nil {
		log.Println("❌ 公司数据解析失败:", err)
		return
	}
	// 统计公司 tiles 数量
	connectedTiles := data.GetConnectedTiles(repository.Rdb, r.ID, tileKey)
	companyData.Tiles = len(connectedTiles)
	companyData.StockTotal--

	// 写回 Hash
	companyUpdateMap := map[string]interface{}{
		"tiles":      companyData.Tiles,
		"stockTotal": companyData.StockTotal,
	}

	if err := repository.Rdb.HSet(repository.Ctx, companyKey, companyUpdateMap).Err(); err != nil {
		log.Println("❌ 写回公司数据失败:", err)
		return
	}

	log.Println("✅ 公司数据已更新:", companyData)

	tileMap, err := data.GetAllRoomTiles(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取房间所有 tile 数据失败:", err)
		return
	}

	for _, tileKey := range connectedTiles {
		tile, ok := tileMap[tileKey]
		if !ok {
			log.Printf("⚠️ tileKey %s 不存在，跳过", tileKey)
			continue
		}

		// 修改归属
		tile.Belong = company

		// 写回 Redis
		if err := data.UpdateTileValue(repository.Rdb, r.ID, tileKey, tile); err != nil {
			log.Printf("❌ 更新 tile %s 失败: %v", tileKey, err)
		} else {
			log.Printf("✅ 成功更新 tile %s 的归属为 %s", tileKey, company)
		}
	}
	// Step 3: 增加玩家的股票数据
	playerStockKey := fmt.Sprintf("room:%s:player:%s:stocks", r.ID, cmd.PlayerID)
	if err := repository.Rdb.HIncrBy(repository.Ctx, playerStockKey, company, 1).Err(); err != nil {
		log.Println("❌ 增加玩家股票失败:", err)
		return
	}
	log.Println("✅ 玩家获得 1 股", company, "股票")

	// Step 4: 清除 createTileKey
	// _ = rdb.Del(repository.Ctx, createTileKey).Err()
	// Step 5:🔥 清除玩家的 tile
	if err := data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusBuyStock); err != nil {
		log.Println("❌ 设置房间状态失败:", err)
	}
}
