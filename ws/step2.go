package ws

import (
	"context"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

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

func handleBuyStockMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	currentPlayer, err := GetCurrentPlayer(rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != playerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	roomInfo, err := GetRoomInfo(rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusBuyStock {
		log.Println("❌ 不是 buyStock 的状态")
		return
	}
	stocks, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		log.Println("❌ 股票数据格式错误")
		return
	}

	totalPrice := 0
	priceMap := make(map[string]int)

	for company, countVal := range stocks {
		count := int(countVal.(float64))

		// 获取股价
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)
		priceStr, err := rdb.HGet(repository.Ctx, companyKey, "stockPrice").Result()
		if err != nil {
			log.Println("❌ 获取股价失败:", company, err)
			return
		}
		price, _ := strconv.Atoi(priceStr)
		priceMap[company] = price
		totalPrice += price * count
	}

	// 遍历更新每个公司
	for company, countVal := range stocks {
		count := int(countVal.(float64))
		for i := 0; i < count; i++ {
			if err := UpdateCompanyStockAndTiles(rdb, roomID, company); err != nil {
				log.Println("❌ 更新公司失败:", err)
				return
			}
		}
	}

	// 再统一扣钱 & 更新玩家股票
	for company, countVal := range stocks {
		count := int(countVal.(float64))
		if err := UpdatePlayerStockAndMoney(rdb, repository.Ctx, roomID, playerID, company, count, priceMap[company]*count); err != nil {
			log.Println("❌ 更新玩家失败:", err)
			return
		}
	}

	err = GiveRandomTileToPlayer(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println("发牌失败:", err)
	}
	// 切换玩家
	if err := SwitchToNextPlayer(rdb, repository.Ctx, roomID, playerID); err != nil {
		log.Println("切换玩家失败:", err)
	}
	// 最后设置房间状态为 setTile
	err = SetGameStatus(rdb, roomID, dto.RoomStatusSetTile)
	if err != nil {
		log.Println("❌ 设置房间状态失败:", err)
	}

	log.Println("✅ 玩家购买股票成功")
}
