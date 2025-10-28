package service

import (
	"acquire-service/dto"
	"acquire-service/entities"
	"acquire-service/repository"
	"acquire-service/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

// GameService 核心游戏服务
type GameService struct {
	rdb *redis.Client
	ctx context.Context
}

// NewGameService 创建游戏服务实例
func NewGameService(rdb *redis.Client) *GameService {
	return &GameService{
		rdb: rdb,
		ctx: context.Background(),
	}
}

// 房间内的所有连接（简化版）
var Rooms = make(map[string][]dto.PlayerConn)
var roomLock sync.Mutex

func SwitchToNextPlayer(rdb *redis.Client, ctx context.Context, roomID, currentID string) error {
	roomLock.Lock()
	defer roomLock.Unlock()

	players, ok := Rooms[roomID]
	if !ok || len(players) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", roomID)
	}

	// 找到当前玩家索引
	var currentIndex int = -1
	for i, pc := range players {
		if pc.PlayerID == currentID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return fmt.Errorf("未找到当前玩家 %s", currentID)
	}

	// 下一个玩家索引（循环）
	nextIndex := (currentIndex + 1) % len(players)
	nextPlayerID := players[nextIndex].PlayerID

	// 设置当前玩家
	if err := SetCurrentPlayer(rdb, ctx, roomID, nextPlayerID); err != nil {
		return fmt.Errorf("切换当前玩家失败: %w", err)
	}

	log.Printf("✅ 已将当前玩家切换为: %s\n", nextPlayerID)
	return nil
}

// 玩家断开连接后，从房间中移除该连接
func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
	roomLock.Lock()
	defer roomLock.Unlock()

	// 遍历查找玩家，并标记为离线
	for i, pc := range Rooms[roomID] {
		if pc.PlayerID == playerID {
			if pc.Conn == conn {
				Rooms[roomID][i].Online = false
				Rooms[roomID][i].Conn = nil // 连接置空，方便回收
				log.Printf("玩家 %s 标记为离线\n", playerID)
			}
			break
		}
	}

	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.RoomStatus {
		SetRoomStatus(repository.Rdb, roomID, false)
	}
	BroadcastToRoom(roomID)
}

// PlaceTile 用于处理将棋子放置到棋盘上：修改 tile 的 belong 字段并更新 Redis，同时从玩家手牌中移除该 tile。
func placeTile(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	// Step 1：下棋
	if err := UpdateTileValue(rdb, roomID, tileKey, dto.Tile{ID: tileKey, Belong: "Blank"}); err != nil {
		return fmt.Errorf("❌ 写入 tile 出错: %w", err)
	}

	// Step 2：从玩家 tile 列表中移除该 tile
	if err := RemovePlayerTile(rdb, ctx, roomID, playerID, tileKey); err != nil {
		return err
	}

	// Step 3: 保存刚刚放置的 tileKey
	if err := SetLastTileKey(rdb, ctx, roomID, playerID, tileKey); err != nil {
		return err
	}

	log.Printf("✅ 玩家 %s 放置棋子 %s 成功\n", playerID, tileKey)
	return nil
}

func handleMergeProcess(
	rdb *redis.Client,
	roomID string,
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

		for _, pc := range Rooms[roomID] {
			playerID := pc.PlayerID
			// 获取该玩家所有股票
			stockMap, err := GetPlayerStocks(rdb, repository.Ctx, roomID, playerID)
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
			if err := AddPlayerMoney(rdb, repository.Ctx, roomID, playerID, money); err != nil {
				log.Println("❌ 累加红利失败:", err)
			}
		}
		tempSettleData[hotel] = dto.SettleData{
			Hoders:    currentCompanyHoders,
			Dividends: dividends,
		}
	}
	// 保存主公司到redis
	err := SetMergeMainCompany(rdb, repository.Ctx, roomID, mainHotel)
	if err != nil {
		return err
	}
	err = SetMergeSettleData(repository.Ctx, rdb, roomID, tempSettleData)
	if err != nil {
		return fmt.Errorf("❌ 保存结算数据失败: %w", err)
	}
	// Step 6：设置状态为“并购清算”
	err = SetGameStatus(rdb, roomID, dto.RoomStatusMergingSettle)
	if err != nil {
		log.Println("❌ 设置房间状态失败:", err)
	}
	log.Printf("✅ 完成酒店[%s]并入[%s]的红利计算和状态更新\n", otherHotel, mainHotel)
	return nil
}

func HandlePostTilePlacement(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	// 第一步：获取公司信息
	companyInfo, err := GetCompanyInfo(rdb, roomID)
	if err != nil {
		return fmt.Errorf("获取公司信息失败: %w", err)
	}

	// 第二步：检查是否有任何公司可购买股票
	for _, info := range companyInfo {
		if tilesCount := info.Tiles; tilesCount > 0 {
			// 有公司可买，设置房间状态为“买股票”
			if err := SetGameStatus(rdb, roomID, dto.RoomStatusBuyStock); err != nil {
				return fmt.Errorf("更新房间状态失败: %w", err)
			}
			return nil
		}
	}
	// 发一张 tile
	if err := GiveRandomTileToPlayer(rdb, repository.Ctx, roomID, playerID); err != nil {
		return fmt.Errorf("发牌失败: %w", err)
	}

	// 切换玩家
	if err := SwitchToNextPlayer(rdb, repository.Ctx, roomID, playerID); err != nil {
		log.Println("切换玩家失败:", err)
	}
	return nil
}

func handleMergingLogic(rdb *redis.Client, roomID string, playerID string, hotelSet map[string]struct{}) error {
	// 统计每个酒店的 tile 数量
	companyInfo, err := GetCompanyInfo(rdb, roomID)
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
			err = SetGameStatus(rdb, roomID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有其他可以合并的公司")
			return nil
		}
		err = SetMergingSelection(rdb, repository.Ctx, roomID, entities.MergingSelection{
			MainCompany:  topHotels,
			OtherCompany: otherHotel,
		})
		if err != nil {
			return err
		}
		err := SetGameStatus(rdb, roomID, dto.RoomStatusMergingSelection)
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
			err = SetGameStatus(rdb, roomID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有其他可以合并的公司")
			return nil
		}
		err = handleMergeProcess(rdb, roomID, mainHotel, otherHotel, hotelTileCount)
		if err != nil {
			return err
		}
	}
	return nil
}

// 检查是否有创建、并购、扩建规则触发
func checkTileTriggerRules(rdb *redis.Client, roomID string, playerID string, tileKey string) error {
	adjTiles := getAdjacentTileKeys(tileKey)
	companySet := make(map[string]struct{})
	blankTileCount := 0

	for _, adjKey := range adjTiles {
		tile, err := GetTileFromRedis(rdb, repository.Ctx, roomID, adjKey)
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
		err := handleMergingLogic(rdb, roomID, playerID, companySet)
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

		connectedTiles := getConnectedTiles(rdb, roomID, tileKey)
		for _, tileKeyBlank := range connectedTiles {
			// 写回 Redis
			if err := UpdateTileValue(rdb, roomID, tileKeyBlank, dto.Tile{ID: tileKeyBlank, Belong: company}); err != nil {
				log.Printf("❌ 更新 tile %s 失败: %v", tileKeyBlank, err)
			} else {
				log.Printf("✅ 成功更新 tile %s 的归属为 %s", tileKeyBlank, company)
			}
		}

		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)
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
		connectedTiles = getConnectedTiles(rdb, roomID, tileKey)
		companyData.Tiles = len(connectedTiles)

		// 写回 Hash
		companyUpdateMap := map[string]interface{}{
			"tiles": companyData.Tiles,
		}
		if err := rdb.HSet(repository.Ctx, companyKey, companyUpdateMap).Err(); err != nil {
			return fmt.Errorf("写回公司数据失败: %w", err)
		}
		log.Println("✅ 公司数据已更新:", companyData)

		err = HandlePostTilePlacement(repository.Rdb, repository.Ctx, roomID, playerID)
		if err != nil {
			log.Println("处理玩家放置 tile 后逻辑失败:", err)
		}
		return nil
	}

	if blankTileCount >= 1 {
		companyInfo, err := GetCompanyInfo(rdb, roomID)
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
			err = SetGameStatus(rdb, roomID, dto.RoomStatusBuyStock)
			if err != nil {
				log.Println("❌ 设置房间状态失败:", err)
			}
			log.Println("没有可以创建的公司")
			return nil
		}

		log.Println("⚠️ 触发创建公司规则！创建一个酒店:")
		// Step 1: 修改房间状态为“创建公司状态”
		SetGameStatus(rdb, roomID, dto.RoomStatusCreateCompany)
		return nil
	}

	err := HandlePostTilePlacement(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println("处理玩家放置 tile 后逻辑失败:", err)
	}
	return nil
}

// 处理玩家放置 tile 消息
func (gs *GameService) HandlePlaceTile(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	msgMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	currentPlayer, err := GetCurrentPlayer(gs.rdb, ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return nil, err
	}
	if currentPlayer != playerID {
		err := fmt.Errorf("不是当前玩家的回合")
		log.Println("❌", err)
		return nil, err
	}

	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return nil, err
	}
	if roomInfo.GameStatus != dto.RoomStatusSetTile {
		err := fmt.Errorf("不是放置 tile 的状态")
		log.Println("❌", err)
		return nil, err
	}

	tileKey, ok := msgMap["payload"].(string)
	if !ok {
		err := fmt.Errorf("无效的 payload")
		log.Println(err)
		return nil, err
	}

	// Step1: 放置棋子
	err = placeTile(gs.rdb, ctx, roomID, playerID, tileKey)
	if err != nil {
		log.Println("放置棋子失败", tileKey)
		return nil, err
	}

	// Step2: 检查 创建公司/并购公司
	err = checkTileTriggerRules(gs.rdb, roomID, playerID, tileKey)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 广播房间消息
	BroadcastToRoom(roomID)
	return nil, nil
}

func (gs *GameService) HandleMergingSelection(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	msgMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	currentPlayer, err := GetCurrentPlayer(gs.rdb, ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return nil, err
	}
	if currentPlayer != playerID {
		err := fmt.Errorf("不是当前玩家的回合")
		log.Println("❌", err)
		return nil, err
	}

	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return nil, err
	}
	if roomInfo.GameStatus != dto.RoomStatusMergingSelection {
		err := fmt.Errorf("不是 merging_selection 的状态")
		log.Println("❌", err)
		return nil, err
	}
	maincompany, ok := msgMap["payload"].(string)
	if !ok {
		err := fmt.Errorf("留下的公司格式错误")
		log.Println("❌", err)
		return nil, err
	}

	mergeSelectionTemp, err := GetMergingSelection(gs.rdb, ctx, roomID)
	if err != nil {
		log.Println("❌ 获取合并选择失败:", err)
		return nil, err
	}
	companyInfo, err := GetCompanyInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return nil, err
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
			mergeSelectionTemp.OtherCompany = removeAtIndex(mergeSelectionTemp.OtherCompany, i)
			continue
		}
		tileCount := info.Tiles
		hotelTileCount[hotel] = tileCount
		if tileCount > maxCount {
			maxCount = tileCount
		}
	}

	err = handleMergeProcess(gs.rdb, roomID, maincompany, mergeSelectionTemp.OtherCompany, hotelTileCount)
	if err != nil {
		log.Println("❌ 处理合并过程失败:", err)
		return nil, err
	}
	return nil, nil
}

func (gs *GameService) HandleMergingSettle(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	msgMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return nil, err
	}
	if roomInfo.GameStatus != dto.RoomStatusMergingSettle {
		err := fmt.Errorf("不是合并的状态")
		log.Println("❌", err)
		return nil, err
	}

	mergeSettleData, err := GetMergeSettleData(ctx, gs.rdb, roomID)
	if err != nil {
		log.Printf("❌ 获取合并数据失败: %v\n", err)
		return nil, err
	}

	playerInHoder := false
	for _, data := range mergeSettleData {
		oldHoders := data.Hoders
		for _, h := range oldHoders {
			if h == playerID {
				playerInHoder = true
			}
		}
	}
	if !playerInHoder {
		err := fmt.Errorf("玩家不在任何合并中")
		log.Println("❌", err)
		return nil, err
	}

	lockKey := fmt.Sprintf("lock:merge_settle:%s", roomID)
	lockValue := uuid.NewString()
	locked, err := gs.rdb.SetNX(ctx, lockKey, lockValue, 5*time.Second).Result()
	if err != nil || !locked {
		err := fmt.Errorf("玩家[%s]尝试结算但加锁失败，可能有人在操作中", playerID)
		log.Printf("⚠️ %v\n", err)
		return nil, err
	}
	defer func() {
		val, err := gs.rdb.Get(ctx, lockKey).Result()
		if err == nil && val == lockValue {
			gs.rdb.Del(ctx, lockKey)
		}
	}()

	payloadRaw := msgMap["payload"]

	// 将 interface{} 编码成 JSON
	payloadBytes, err := json.Marshal(payloadRaw)
	if err != nil {
		log.Println("❌ payload 编码失败:", err)
		return nil, err
	}

	// 反序列化为结构体切片
	var settleActions []dto.MergingSettleItem
	if err := json.Unmarshal(payloadBytes, &settleActions); err != nil {
		log.Println("❌ payload 反序列化失败:", err)
		return nil, err
	}

	companyInfo, err := GetCompanyInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取公司信息失败:", err)
		return nil, err
	}

	stockMap, err := GetPlayerStocks(gs.rdb, ctx, roomID, playerID)
	if err != nil {
		log.Printf("❌ 获取玩家[%s]股票失败: %v\n", playerID, err)
		return nil, err
	}

	mergeMainCompany, err := GetMergeMainCompany(gs.rdb, ctx, roomID)
	if err != nil {
		log.Printf("❌ 获取合并主公司失败: %v\n", err)
		return nil, err
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
			if err := AddPlayerMoney(gs.rdb, ctx, roomID, playerID, money); err != nil {
				log.Printf("❌ 扣除玩家[%s]股票失败: %v\n", playerID, err)
				return nil, err
			}
		}

		if exchangeAmount > 0 {
			// 修改股票持仓
			stockMap[mergeMainCompany] += exchangeAmount / 2
			stockMap[item.Company] -= exchangeAmount
		}
	}

	err = SetPlayerStocks(gs.rdb, ctx, roomID, playerID, stockMap)
	if err != nil {
		log.Printf("❌ 保存玩家[%s]股票失败: %v\n", playerID, err)
		return nil, err
	}

	allHodersCleared := true
	// 移除 Hoders 中的 playerID
	for key, data := range mergeSettleData {
		oldHoders := data.Hoders
		newHoders := make([]string, 0)
		for _, h := range oldHoders {
			if h != playerID {
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
		lastTile, err := GetLastTileKey(gs.rdb, ctx, roomID)
		if err != nil {
			log.Printf("❌ 获取当前创建公司 tile key 失败: %v\n", err)
			return nil, err
		}

		connTile := getConnectedTiles(gs.rdb, roomID, lastTile)
		connTileSet := make(map[string]struct{})
		for _, id := range connTile {
			connTileSet[id] = struct{}{}
		}

		tileMap, err := GetAllRoomTiles(gs.rdb, roomID)
		if err != nil {
			log.Printf("❌ 获取房间 tile 信息失败: %v\n", err)
			return nil, err
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

		err = SetAllRoomTiles(gs.rdb, roomID, tileMap)
		if err != nil {
			log.Printf("❌ 保存房间 tile 信息失败: %v\n", err)
			return nil, err
		}
		if err != nil {
			log.Printf("❌ 获取最后一个 tile key 失败: %v\n", err)
			return nil, err
		}
		adj := getAdjacentTileKeys(lastTile)
		for _, key := range adj {
			tile, err := GetTileFromRedis(gs.rdb, ctx, roomID, key)
			if err != nil {
				log.Printf("❌ 获取 tileBelong 失败: %v\n", err)
				return nil, err
			}
			if tile.Belong == "Blank" {
				tile.Belong = mergeMainCompany
				err = UpdateTileValue(gs.rdb, roomID, key, tile)
				if err != nil {
					log.Printf("❌ 更新 tileBelong 失败: %v\n", err)
					return nil, err
				}
			}
		}

		err = SetGameStatus(gs.rdb, roomID, dto.RoomStatusBuyStock)
		if err != nil {
			log.Printf("❌ 设置游戏状态失败: %v\n", err)
			return nil, err
		}
		if err := SetMergeSettleData(ctx, gs.rdb, roomID, map[string]dto.SettleData{}); err != nil {
			log.Printf("❌ 保存结算数据失败: %v\n", err)
			return nil, err
		}
	} else {
		// 保存结果
		if err := SetMergeSettleData(ctx, gs.rdb, roomID, mergeSettleData); err != nil {
			log.Printf("❌ 保存结算数据失败: %v\n", err)
			return nil, err
		}
	}
	BroadcastToRoom(roomID)
	return nil, nil
}

func (gs *GameService) HandleCreateCompany(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	msgMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	currentPlayer, err := GetCurrentPlayer(gs.rdb, ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return nil, err
	}
	if currentPlayer != playerID {
		err := fmt.Errorf("不是当前玩家的回合")
		log.Println("❌", err)
		return nil, err
	}

	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return nil, err
	}
	if roomInfo.GameStatus != dto.RoomStatusCreateCompany {
		err := fmt.Errorf("不是创建公司的状态")
		log.Println("❌", err)
		return nil, err
	}

	company, ok := msgMap["payload"].(string)
	if !ok {
		err := fmt.Errorf("无效的 payload")
		log.Println("❌", err)
		return nil, err
	}
	log.Println("✅ 收到 create_company 消息，目标 company:", company)

	// Step 1: 取出 createTileKey
	createTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)
	tileKey, err := gs.rdb.Get(ctx, createTileKey).Result()
	if err != nil {
		log.Println("❌ 获取 createTileKey 失败:", err)
		return nil, err
	}
	log.Println("✅ 创建公司使用的 tileKey:", tileKey)

	// Step 2: 修改公司数据（仍用 Hash 类型保存）
	companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)

	// 获取公司 Hash 数据
	companyMap, err := gs.rdb.HGetAll(ctx, companyKey).Result()
	if err != nil {
		log.Println("❌ 获取公司 Hash 数据失败:", err)
		return nil, err
	}
	if len(companyMap) == 0 {
		err := fmt.Errorf("公司 Hash 数据为空")
		log.Println("❌", err)
		return nil, err
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
		return nil, err
	}
	// 统计公司 tiles 数量
	connectedTiles := getConnectedTiles(gs.rdb, roomID, tileKey)
	companyData.Tiles = len(connectedTiles)
	companyData.StockTotal--

	// 写回 Hash
	companyUpdateMap := map[string]interface{}{
		"tiles":      companyData.Tiles,
		"stockTotal": companyData.StockTotal,
	}

	if err := gs.rdb.HSet(ctx, companyKey, companyUpdateMap).Err(); err != nil {
		log.Println("❌ 写回公司数据失败:", err)
		return nil, err
	}

	log.Println("✅ 公司数据已更新:", companyData)

	tileMap, err := GetAllRoomTiles(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间所有 tile 数据失败:", err)
		return nil, err
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
		if err := UpdateTileValue(gs.rdb, roomID, tileKey, tile); err != nil {
			log.Printf("❌ 更新 tile %s 失败: %v", tileKey, err)
		} else {
			log.Printf("✅ 成功更新 tile %s 的归属为 %s", tileKey, company)
		}
	}
	// Step 3: 增加玩家的股票数据
	playerStockKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	if err := gs.rdb.HIncrBy(ctx, playerStockKey, company, 1).Err(); err != nil {
		log.Println("❌ 增加玩家股票失败:", err)
		return nil, err
	}
	log.Println("✅ 玩家获得 1 股", company, "股票")

	// Step 4: 清除 createTileKey
	// _ = rdb.Del(repository.Ctx, createTileKey).Err()
	// Step 5:🔥 清除玩家的 tile
	SetGameStatus(gs.rdb, roomID, dto.RoomStatusBuyStock)
	BroadcastToRoom(roomID)
	return nil, nil
}

// UpdateCompanyStockAndTiles 更新公司数据（stockTotal 减少）
func UpdateCompanyStockAndTiles(rdb *redis.Client, roomID string, company string) error {
	companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)

	companyMap, err := rdb.HGetAll(repository.Ctx, companyKey).Result()
	if err != nil || len(companyMap) == 0 {
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

	// 更新 stockTotal（每次只减1）
	if companyData.StockTotal <= 0 {
		return fmt.Errorf("公司股票已售罄")
	}
	companyData.StockTotal--

	// 写回更新
	update := map[string]interface{}{
		"stockTotal": companyData.StockTotal,
	}

	if err := rdb.HSet(repository.Ctx, companyKey, update).Err(); err != nil {
		return fmt.Errorf("更新公司数据失败: %w", err)
	}

	log.Println("✅ 公司已更新:", company, update)
	return nil
}

// UpdatePlayerStockAndMoney 更新玩家数据
func UpdatePlayerStockAndMoney(rdb *redis.Client, ctx context.Context, roomID string, playerID string, company string, stockCount int, totalPrice int) error {
	// 获取当前金额
	playerInfo, err := GetPlayerInfoField(rdb, ctx, roomID, playerID, "money")
	if err != nil {
		return fmt.Errorf("获取玩家金额失败: %w", err)
	}
	money := playerInfo.Money

	if money < totalPrice {
		return fmt.Errorf("余额不足，购买失败")
	}
	newMoney := money - totalPrice

	if err := SetPlayerInfoField(rdb, ctx, roomID, playerID, "money", newMoney); err != nil {
		return fmt.Errorf("更新余额失败: %w", err)
	}

	// 获取玩家现有股票
	stockMap, err := GetPlayerStocks(rdb, ctx, roomID, playerID)
	if err != nil {
		log.Printf("❌ 获取玩家[%s]股票失败: %v\n", playerID, err)
		return err
	}

	// 解析已有股票数量
	existingStock := stockMap[company]
	stockMap[company] = existingStock + stockCount

	stockMapInterface := make(map[string]int, len(stockMap))
	for k, v := range stockMap {
		stockMapInterface[k] = v
	}
	// 写回玩家股票信息
	err = SetPlayerStocks(rdb, ctx, roomID, playerID, stockMapInterface)
	if err != nil {
		log.Println("❌ 写入玩家股票失败:", err)
		return fmt.Errorf("写入玩家股票失败: %w", err)
	}

	log.Println("✅ 玩家数据已更新")
	return nil
}

type BuyStockRequest struct {
	Stocks map[string]float64 `json:"stocks"`
}

func (gs *GameService) HandleBuyStock(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	msgMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data format")
	}

	currentPlayer, err := GetCurrentPlayer(gs.rdb, ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return nil, err
	}
	if currentPlayer != playerID {
		err := fmt.Errorf("不是当前玩家的回合")
		log.Println("❌", err)
		return nil, err
	}

	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return nil, err
	}
	if roomInfo.GameStatus != dto.RoomStatusBuyStock {
		err := fmt.Errorf("不是 buyStock 的状态")
		log.Println("❌", err)
		return nil, err
	}
	payloadMap, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("股票数据格式错误")
		log.Println("❌", err)
		return nil, err
	}

	stocks := make(map[string]int)

	for k, v := range payloadMap {
		// 判断 v 是不是 float64，再转换成 int
		if f, ok := v.(float64); ok {
			stocks[k] = int(f)
		} else {
			log.Printf("⚠️ 股票值类型错误: key=%s val=%v type=%T\n", k, v, v)
		}
	}

	totalPrice := 0
	priceMap := make(map[string]int)

	for company, countVal := range stocks {
		count := countVal

		// 获取股价
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)
		priceStr, err := gs.rdb.HGet(ctx, companyKey, "stockPrice").Result()
		if err != nil {
			log.Println("❌ 获取股价失败:", company, err)
			return nil, err
		}
		price, _ := strconv.Atoi(priceStr)
		priceMap[company] = price
		totalPrice += price * count
	}

	// 遍历更新每个公司
	for company, countVal := range stocks {
		count := countVal
		for i := 0; i < count; i++ {
			if err := UpdateCompanyStockAndTiles(gs.rdb, roomID, company); err != nil {
				log.Println("❌ 更新公司失败:", err)
				return nil, err
			}
		}
	}

	// 再统一扣钱 & 更新玩家股票
	for company, countVal := range stocks {
		count := countVal
		if err := UpdatePlayerStockAndMoney(gs.rdb, ctx, roomID, playerID, company, count, priceMap[company]*count); err != nil {
			log.Println("❌ 更新玩家失败:", err)
			return nil, err
		}
	}

	err = GiveRandomTileToPlayer(gs.rdb, ctx, roomID, playerID)
	if err != nil {
		log.Println("发牌失败:", err)
		return nil, err
	}

	// 切换玩家
	if err := SwitchToNextPlayer(gs.rdb, ctx, roomID, playerID); err != nil {
		log.Println("切换玩家失败:", err)
		return nil, err
	}

	// 最后设置房间状态为 setTile
	err = SetGameStatus(gs.rdb, roomID, dto.RoomStatusSetTile)
	if err != nil {
		log.Println("❌ 设置房间状态失败:", err)
		return nil, err
	}

	log.Println("✅ 玩家购买股票成功")

	// 广播房间消息
	BroadcastToRoom(roomID)
	return nil, nil
}

func GiveRandomTileToPlayer(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	allTiles, err := generateAvailableTiles(roomID)
	if err != nil {
		return fmt.Errorf("生成可用 tiles 失败: %w", err)
	}

	if len(allTiles) == 0 {
		log.Println("❌ 没有可用的 tiles")
		return nil
	}

	rand.Shuffle(len(allTiles), func(i, j int) {
		allTiles[i], allTiles[j] = allTiles[j], allTiles[i]
	})

	// 使用 SafeSlice 安全获取一张 tile
	selected := utils.SafeSlice(allTiles, 1)
	if len(selected) == 0 {
		return fmt.Errorf("无法为玩家分配 tile")
	}

	// 添加到玩家 tiles 中
	if err := AddPlayerTile(rdb, ctx, roomID, playerID, selected[0]); err != nil {
		return fmt.Errorf("添加 tile 失败: %w", err)
	}

	log.Printf("✅ 玩家 %s 获得 tile：%s\n", playerID, selected[0])
	return nil
}

func handlePlayAudioMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	audioType, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("❌ 消息格式错误")
		return
	}

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("❌ 编码 JSON 失败:", err)
		return
	}

	for _, pc := range Rooms[roomID] {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Printf("❌ 向玩家 %s 发送音频消息失败: %v\n", pc.PlayerID, err)
			}
		}
	}
}

func (gs *GameService) HandleRestartGame(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	// 重置上次落子
	if err := SetLastTileKey(gs.rdb, ctx, roomID, playerID, ""); err != nil {
		log.Println("❌ 设置最后放置的 tile 失败:", err)
		return nil, err
	}
	// 重置游戏状态
	SetGameStatus(gs.rdb, roomID, dto.RoomStatusSetTile)
	// 重置tiles
	tile, err := GetAllRoomTiles(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 获取所有 tile 失败:", err)
		return nil, err
	}
	for tileKey, tileInfo := range tile {
		tileInfo.Belong = ""
		tile[tileKey] = tileInfo
	}
	err = SetAllRoomTiles(gs.rdb, roomID, tile)
	if err != nil {
		log.Println("❌ 重置 tile 失败:", err)
		return nil, err
	}

	for _, pc := range Rooms[roomID] {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		err = SetPlayerInfoField(gs.rdb, ctx, roomID, playerID, "money", 6000)
		if err != nil {
			log.Println("设置玩家信息失败:", err)
		}

		allTiles, err := generateAvailableTiles(roomID)
		if err != nil {
			log.Println(err)
		}
		rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })
		playerTiles := utils.SafeSlice(allTiles, 5)
		err = SetPlayerTiles(gs.rdb, ctx, roomID, playerID, playerTiles)
		if err != nil {
			log.Println(err)
		}
		companyIDs, err := getCompanyIDs(roomID)
		if err != nil {
			log.Println("获取公司ID失败:", err)
			return nil, err
		}
		// 3.2 初始化玩家股票为0
		playerStocks := make(map[string]int)
		for _, company := range companyIDs {
			playerStocks[company] = 0
		}
		err = SetPlayerStocks(gs.rdb, ctx, roomID, playerID, playerStocks)
		if err != nil {
			log.Println("写入玩家股票失败:", err)
		}
	}

	// ... 继续原有逻辑，将所有 repository.Rdb 改为 gs.rdb，repository.Ctx 改为 ctx

	startKey := fmt.Sprintf("room:%s:game_start_time", roomID)
	gs.rdb.Set(ctx, startKey, time.Now().Format("20060102_150405"), 0)

	time.Sleep(2 * time.Second)

	BroadcastToRoom(roomID)
	return nil, nil
}

func (gs *GameService) HandleGameEnd(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	err := SetGameStatus(gs.rdb, roomID, dto.RoomStatusEnd)
	if err != nil {
		log.Println("Error setting game status:", err)
		return nil, err
	}

	logPath := getGameLogFilePath(roomID)
	log.Println("✅ 游戏日志保存于:", logPath)

	BroadcastToRoom(roomID)
	return nil, nil
}
func WriteGameLog(roomID, playerID string, roomInfo *entities.RoomInfo, msg map[string]interface{}) {
	go func() {
		logPath := getGameLogFilePath(roomID)

		// 确保目录存在
		if err := os.MkdirAll(path.Dir(logPath), 0755); err != nil {
			log.Println("❌ 创建日志目录失败:", err)
			return
		}

		entry := map[string]interface{}{
			"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
			"result":     msg["result"],
			"roomInfo":   roomInfo,
			"playerID":   playerID,
			"playerData": msg["playerData"],
			"roomData":   msg["roomData"],
			"tempData":   msg["tempData"],
		}

		jsonEntry, err := json.Marshal(entry)
		if err != nil {
			log.Println("❌ 序列化日志 entry 失败:", err)
			return
		}

		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Println("❌ 打开游戏日志文件失败:", err)
			return
		}
		defer f.Close()

		jsonEntry = append(jsonEntry, ',')

		if _, err := f.Write(jsonEntry); err != nil {
			log.Println("❌ 写入日志失败:", err)
			return
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			log.Println("❌ 写入换行失败:", err)
		}
	}()
}

// 向该客户端发送同步消息
func SyncRoomMessage(conn dto.ConnInterface, roomID string, playerID string, result map[string]int) error {
	rdb := repository.Rdb
	ctx := repository.Ctx

	// ------- 构造 Redis Key -------
	infoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", roomID)
	companyIDsKey := fmt.Sprintf("room:%s:company_ids", roomID)
	lastTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)

	// ------- 第一次 pipeline：玩家、房间、tile 基础数据 -------
	pipe := rdb.Pipeline()
	infoCmd := pipe.HGetAll(ctx, infoKey)
	tilesCmd := pipe.LRange(ctx, tilesKey, 0, -1)
	currentPlayerCmd := pipe.Get(ctx, currentPlayerKey)
	companyIDsCmd := pipe.SMembers(ctx, companyIDsKey)
	lastTileKeyCmd := pipe.Get(ctx, lastTileKey)

	// 执行 pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("❌ Redis pipeline 执行失败: %w", err)
	}

	// ------- 提取结果 -------
	info := infoCmd.Val()
	tiles := tilesCmd.Val()
	currentPlayer := currentPlayerCmd.Val()
	companyIDs := companyIDsCmd.Val()
	lastTile := lastTileKeyCmd.Val()

	// ------- 第二次 pipeline：批量获取所有公司信息 -------
	pipe2 := rdb.Pipeline()
	companyCmds := make(map[string]*redis.StringStringMapCmd)

	for _, companyID := range companyIDs {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)
		companyCmds[companyID] = pipe2.HGetAll(ctx, companyKey)
	}

	_, err = pipe2.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("❌ 获取公司信息 pipeline 执行失败: %w", err)
	}

	companyInfo, err := GetCompanyInfo(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取公司信息失败: %w", err)
	}

	roomInfo, err := GetRoomInfo(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间信息失败: %w", err)
	}

	// ------- 其他 Redis 相关调用 -------
	tileMap, err := GetAllRoomTiles(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间 tile 信息失败: %w", err)
	}

	merge_main_company_temp, err := GetMergeMainCompany(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并主公司信息失败: %w", err)
	}

	merge_selection_temp, err := GetMergingSelection(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并选择信息失败: %w", err)
	}

	mergeSettleData, err := GetMergeSettleData(ctx, rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并结算信息失败: %w", err)
	}

	stocks, err := GetPlayerStocks(rdb, ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("❌ 获取玩家股票信息失败: %w", err)
	}

	// ------- 组装消息 -------
	msg := map[string]interface{}{
		"type":     "sync",
		"result":   result,
		"playerId": playerID,
		"playerData": map[string]interface{}{
			"info":   info,
			"stocks": stocks,
			"tiles":  tiles,
		},
		"roomData": map[string]interface{}{
			"companyInfo":   companyInfo,
			"currentPlayer": currentPlayer,
			"roomInfo":      roomInfo,
			"tiles":         tileMap,
		},
		"tempData": map[string]interface{}{
			"last_tile_key":           lastTile,
			"merge_main_company_temp": merge_main_company_temp,
			"merge_selection_temp":    merge_selection_temp,
			"mergeSettleData":         mergeSettleData,
		},
	}

	// ------- 发送 WebSocket 消息 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}
	if playerID == currentPlayer {
		WriteGameLog(roomID, playerID, roomInfo, msg)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 广播消息给房间内所有连接成功的玩家
func BroadcastToRoom(roomID string) {
	companyInfoMap, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("获取公司信息失败:", err)
		return
	}

	tileMap, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		log.Println("获取所有 tile 失败:", err)
		return
	}
	allTileMap := make(map[string]int)
	for _, tile := range tileMap {
		if tile.Belong != "" && tile.Belong != "Blank" {
			allTileMap[tile.Belong] = allTileMap[tile.Belong] + 1
		}
	}

	allStockMap := make(map[string]int)
	for _, pc := range Rooms[roomID] {
		stockMap, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家[%s]股票失败: %v\n", pc.PlayerID, err)
			return
		}
		for stockID, stockCount := range stockMap {
			allStockMap[stockID] += stockCount
		}
	}

	for companyName, info := range companyInfoMap {
		stockInfo := utils.GetStockInfo(companyName, allTileMap[companyName])
		stockLeft := 25 - allStockMap[companyName]
		info.StockTotal = stockLeft
		info.Tiles = allTileMap[companyName]
		info.StockPrice = stockInfo.Price
		companyInfoMap[companyName] = info
	}

	err = SetCompanyInfo(repository.Rdb, roomID, companyInfoMap)
	if err != nil {
		log.Println("❌ 设置公司信息失败:", err)
		return
	}

	result := make(map[string]int)
	for _, pc := range Rooms[roomID] {
		playerStocks, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家[%s]股票失败: %v\n", pc.PlayerID, err)
			continue
		}
		playerInfo, err := GetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, pc.PlayerID, "money")
		if err != nil {
			log.Printf("❌ 获取玩家[%s]金钱失败: %v\n", pc.PlayerID, err)
			continue
		}
		result[pc.PlayerID] = CalculateTotalValue(playerStocks, companyInfoMap) + playerInfo.Money
	}

	for _, pc := range Rooms[roomID] {
		if pc.Online {
			// 尝试发送消息
			if err := SyncRoomMessage(pc.Conn, roomID, pc.PlayerID, result); err != nil {
				log.Println("广播失败，移除连接:", pc.PlayerID)
				pc.Conn.Close()
			}
		}
	}
}
