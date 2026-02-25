package roompkg

import (
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/utils"
	"sort"
	"time"

	"math/rand/v2"
)

type VirtualConn struct {
	Room *domain.Room
}

// WriteMessage 实现ConnInterface接口
func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	MaybeRunAIIfNeeded(v.Room, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, fmt.Errorf("virtual connection cannot read")
}

// Close 实现ConnInterface接口
func (v *VirtualConn) Close() error {
	// AI玩家不需要实际关闭连接
	return nil
}

func chooseTileForAI(room *domain.Room, playerID string) string {
	tiles := room.State.Players[playerID].Tiles
	allTiles := room.State.BoardTiles

	// 遍历 AI 玩家拥有的 tiles
	for _, tileID := range tiles {
		neighbors := data.GetAdjacentTileKeys(tileID)
		for _, nID := range neighbors {
			if neighborTile, ok := allTiles[nID]; ok && neighborTile.Belong != "" {
				return tileID
			}
		}
	}

	return tiles[rand.IntN(len(tiles))]
}

func chooseCompanyForAI(r *domain.Room) string {
	// 过滤掉已创建的公司
	var uncreated []string
	for company, info := range r.State.Companies {
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
		return p1[rand.IntN(len(p1))]
	}
	if len(p2) > 0 {
		return p2[rand.IntN(len(p2))]
	}
	return p3[rand.IntN(len(p3))]
}

func chooseStocksToBuyForAI(r *domain.Room, playerID string) map[string]int {
	playerInfo := r.State.Players[playerID]
	money := playerInfo.Money
	playerStock := playerInfo.Stocks

	// 收集可购买的公司（已创建，且有库存，且价格不超过总金额）
	type candidate struct {
		Name   string
		Price  int
		Remain int
	}
	var options []candidate
	for name, info := range r.State.Companies {
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

func chooseMergingSettleForAI(r *domain.Room, playerID string) []domain.MergingSettleItem {
	result := []domain.MergingSettleItem{}

	for companyKey := range r.State.MergeSettleData {
		count := r.State.Players[playerID].Stocks[companyKey]
		if count == 0 {
			continue
		}
		mainCompanyInfo := r.State.Companies[r.State.MergeMainCompany]
		company := r.State.Companies[companyKey]

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

		result = append(result, domain.MergingSettleItem{
			Company:        companyKey,
			SellAmount:     sellAmount,
			ExchangeAmount: exchangeAmount,
		})
	}

	return result
}

func chooseMergingSelectionForAI(r *domain.Room, playerID string, mainCompany []string) string {
	res := ""
	max := -1
	for _, companyKey := range mainCompany {
		stockInUse := 25 - r.State.Companies[companyKey].StockTotal
		if stockInUse == 0 {
			continue // 避免除以 0
		}
		num := r.State.Players[playerID].Stocks[companyKey] / stockInUse
		if num > max {
			max = num
			res = companyKey
		}
	}

	return res
}

func MaybeRunAIIfNeeded(r *domain.Room, message []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		utils.Error("AI 消息格式错误", utils.F("room_id", r.ID), utils.F("error", err))
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

	gameStatusStr, ok := roomInfo["roomStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := domain.RoomStatus(gameStatusStr)

	playerId, ok := msg["playerId"].(string)
	if !ok || playerId == "" || (playerId != currentPlayerID && gameStatus != domain.RoomStatusMergingSettle) {
		return false
	}
	// 判断是否是 AI 玩家
	if gameStatus != domain.RoomStatusMergingSettle {
		isAI := false

		if r.Connections[currentPlayerID].AI {
			isAI = true
			utils.Info("检测到被替换为AI的玩家", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID))
		}

		if !isAI {
			utils.Info("当前玩家 %s 不是 AI 玩家", utils.F("player_id", currentPlayerID))
			return false
		}
	}

	// 提取临时数据（合并选择）
	tempData, ok := msg["tempData"].(map[string]interface{})
	if !ok {
		utils.Error("tempData 类型错误", utils.F("room_id", r.ID))
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
	if gameStatus == domain.RoomStatusMergingSettle {
		mergeSettleData := r.State.MergeSettleData

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
			utils.Warn("外层校验玩家不在任何合并中", utils.F("room_id", r.ID), utils.F("player_id", playerId))
			return false
		}
	}
	tiles := r.State.BoardTiles
	isAllTileUsed := true
	for _, tile := range tiles {
		if tile.Belong == "" {
			isAllTileUsed = false
		}
	}
	if isAllTileUsed {
		utils.Error("所有 tile 已被使用", utils.F("room_id", r.ID), utils.F("player_id", playerId))
	}

	utils.Info("当前是 AI 玩家的回合，准备延迟执行 AI 行动", utils.F("room_id", r.ID), utils.F("player_id", playerId), utils.F("game_status", gameStatus))

	// ---------- 在协程中延迟执行 ----------
	go func() {
		time.Sleep(5 * time.Second)

		conn := &VirtualConn{Room: r}
		var aiMsg map[string]interface{}

		switch gameStatus {
		case "setTile":
			tile := chooseTileForAI(r, currentPlayerID)
			if tile == "" {
				utils.Error("AI 未选择有效 tile", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID))
				return
			}
			utils.Info("AI 选择 tile", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("tile", tile))
			aiMsg = map[string]interface{}{
				"type":    "game_place_tile",
				"payload": map[string]interface{}{"tileKey": tile},
			}
		case "createCompany":
			company := chooseCompanyForAI(r)
			if company == "" {
				utils.Error("AI 未选择有效公司", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID))
				return
			}
			utils.Info("AI 选择公司", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("company", company))
			aiMsg = map[string]interface{}{
				"type":    "game_create_company",
				"payload": map[string]interface{}{"company": company},
			}
		case "buyStock":
			stocks := chooseStocksToBuyForAI(r, currentPlayerID)
			utils.Info("AI 选择购买股票", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("stocks", stocks))
			aiMsg = map[string]interface{}{
				"type":    "game_buy_stock",
				"payload": map[string]interface{}{"stocks": stocks},
			}
		case "mergingSelection":
			selection := chooseMergingSelectionForAI(r, currentPlayerID, mainCompany)
			utils.Info("AI 选择合并公司", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("selection", selection))
			aiMsg = map[string]interface{}{
				"type":    "game_merging_selection",
				"payload": map[string]interface{}{"mainCompany": selection},
			}
		case "mergingSettle":
			settle := chooseMergingSettleForAI(r, playerId)
			utils.Info("AI 选择合并结算", utils.F("room_id", r.ID), utils.F("player_id", playerId), utils.F("settle", settle))
			aiMsg = map[string]interface{}{
				"type":    "game_merging_settle",
				"payload": map[string]interface{}{"actions": settle},
			}
		default:
			utils.Warn("当前状态未定义 AI 行为", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("game_status", gameStatus))
			return
		}

		// 加入 playerID 然后交给 handler 执行
		// 将 AI 消息转换为 Command 格式，和玩家一样通过通道传递
		payload, err := json.Marshal(aiMsg["payload"])
		if err != nil {
			utils.Error("AI 消息序列化失败", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("error", err))
			return
		}
		utils.Info("AI 发送消息", utils.F("room_id", r.ID), utils.F("player_id", currentPlayerID), utils.F("message", string(payload)))

		// 向房间的命令通道发送消息，和玩家一样的处理方式
		r.CmdCh <- domain.Command{
			Type:     aiMsg["type"].(string),
			PlayerID: playerId,
			Payload:  payload,
			Conn:     conn,
		}
	}()

	return true
}
