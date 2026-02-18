package game

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"strconv"

	"github.com/go-redis/redis/v8"
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

	utils.Info("公司已更新", utils.F("company", company), utils.F("update", update))
	return nil
}

// UpdatePlayerStockAndMoney 更新玩家数据
func UpdatePlayerStockAndMoney(rdb *redis.Client, ctx context.Context, roomID string, playerID string, company string, stockCount int, totalPrice int) error {
	// 获取当前金额
	playerInfo, err := data.GetPlayerInfoField(rdb, ctx, roomID, playerID, "money")
	if err != nil {
		return fmt.Errorf("获取玩家金额失败: %w", err)
	}
	money := playerInfo.Money

	if money < totalPrice {
		return fmt.Errorf("余额不足，购买失败")
	}
	newMoney := money - totalPrice

	if err := data.SetPlayerInfoField(rdb, ctx, roomID, playerID, "money", newMoney); err != nil {
		return fmt.Errorf("更新余额失败: %w", err)
	}

	// 获取玩家现有股票
	stockMap, err := data.GetPlayerStocks(rdb, ctx, roomID, playerID)
	if err != nil {
		utils.Error("获取玩家股票失败", utils.F("player_id", playerID), utils.F("error", err))
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
	err = data.SetPlayerStocks(rdb, ctx, roomID, playerID, stockMapInterface)
	if err != nil {
		utils.Error("写入玩家股票失败", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("error", err))
		return fmt.Errorf("写入玩家股票失败: %w", err)
	}

	utils.Info("玩家数据已更新", utils.F("room_id", roomID), utils.F("player_id", playerID), utils.F("company", company), utils.F("stock_count", stockCount), utils.F("total_price", totalPrice), utils.F("money_after", newMoney))
	return nil
}

type BuyStockPayload struct {
	Stocks map[string]int `json:"stocks"`
}

func HandleBuyStockMessage(r *domain.Room, cmd domain.Command) {
	var p BuyStockPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		utils.Warn("无效的 buy_stock payload", utils.F("error", err))
		return
	}

	stocks := p.Stocks

	currentPlayer, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, r.ID)
	if err != nil {
		utils.Error("获取当前玩家失败", utils.F("error", err))
		return
	}
	if currentPlayer != cmd.PlayerID {
		utils.Warn("不是当前玩家的回合", utils.F("player_id", cmd.PlayerID), utils.F("current_player", currentPlayer))
		return
	}

	roomInfo, err := data.GetRoomInfo(repository.Rdb, r.ID)
	if err != nil {
		utils.Error("获取房间信息失败", utils.F("error", err))
		return
	}
	if roomInfo.GameStatus != dto.RoomStatusBuyStock {
		utils.Warn("不是 buyStock 的状态", utils.F("status", roomInfo.GameStatus))
		return
	}

	totalPrice := 0
	priceMap := make(map[string]int)

	for company, countVal := range stocks {
		count := countVal

		// 获取股价
		companyKey := fmt.Sprintf("room:%s:company:%s", r.ID, company)
		priceStr, err := repository.Rdb.HGet(repository.Ctx, companyKey, "stockPrice").Result()
		if err != nil {
			utils.Error("获取股价失败", utils.F("company", company), utils.F("error", err))
			return
		}
		price, _ := strconv.Atoi(priceStr)
		priceMap[company] = price
		totalPrice += price * count
	}

	// 遍历更新每个公司
	for company, countVal := range stocks {
		count := countVal
		for i := 0; i < count; i++ {
			if err := UpdateCompanyStockAndTiles(repository.Rdb, r.ID, company); err != nil {
				utils.Error("更新公司失败", utils.F("company", company), utils.F("error", err))
				return
			}
		}
	}

	// 再统一扣钱 & 更新玩家股票
	for company, countVal := range stocks {
		count := countVal
		if err := UpdatePlayerStockAndMoney(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, company, count, priceMap[company]*count); err != nil {
			utils.Error("更新玩家失败", utils.F("player_id", cmd.PlayerID), utils.F("company", company), utils.F("error", err))
			return
		}
	}

	err = GiveRandomTileToPlayer(repository.Rdb, repository.Ctx, r, cmd.PlayerID)
	if err != nil {
		utils.Warn("发牌失败", utils.F("player_id", cmd.PlayerID), utils.F("error", err))
	}
	// 切换玩家
	if err := SwitchToNextPlayer(repository.Rdb, repository.Ctx, r, cmd.PlayerID); err != nil {
		utils.Error("切换玩家失败", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
	}
	// 最后设置房间状态为 setTile
	err = data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusSetTile)
	if err != nil {
		utils.Error("设置房间状态失败", utils.F("room_id", r.ID), utils.F("error", err))
	}

	utils.Info("玩家购买股票成功", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("total_price", totalPrice))
}
