package roompkg

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/arrayutil"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"

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

	if len(tiles) == 0 {
		return ""
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
		if arrayutil.StringInSlice(c, priority1) {
			p1 = append(p1, c)
		} else if arrayutil.StringInSlice(c, priority2) {
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
	if len(p3) > 0 {
		return p3[rand.IntN(len(p3))]
	}
	return ""
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

// buildAIActionMsg 根据当前 RoomStatus 选出"AI/超时"应当投递的命令 type+payload。
// 返回 ok=false 表示该状态下没有可投递的动作。
//   - mergingSelection 阶段需要的 mainCompany 候选优先取 explicitMainCompany（来自前端 tempData），
//     若为空则回退到 r.State.MergingSelection.MainCompany（用于 turn_timeout 路径）。
func buildAIActionMsg(r *domain.Room, playerID string, status domain.RoomStatus, explicitMainCompany []string) (cmdType string, payload []byte, ok bool) {
	switch status {
	case domain.RoomStatusSetTile:
		tile := chooseTileForAI(r, playerID)
		if tile == "" {
			return "", nil, false
		}
		data, err := json.Marshal(map[string]interface{}{"tileKey": tile})
		if err != nil {
			return "", nil, false
		}
		return "game_place_tile", data, true
	case domain.RoomStatusCreateCompany:
		company := chooseCompanyForAI(r)
		if company == "" {
			return "", nil, false
		}
		data, err := json.Marshal(map[string]interface{}{"company": company})
		if err != nil {
			return "", nil, false
		}
		return "game_create_company", data, true
	case domain.RoomStatusBuyStock:
		stocks := chooseStocksToBuyForAI(r, playerID)
		data, err := json.Marshal(map[string]interface{}{"stocks": stocks})
		if err != nil {
			return "", nil, false
		}
		return "game_buy_stock", data, true
	case domain.RoomStatusMergingSelection:
		mainCompany := explicitMainCompany
		if len(mainCompany) == 0 {
			mainCompany = r.State.MergingSelection.MainCompany
		}
		selection := chooseMergingSelectionForAI(r, playerID, mainCompany)
		if selection == "" {
			return "", nil, false
		}
		data, err := json.Marshal(map[string]interface{}{"mainCompany": selection})
		if err != nil {
			return "", nil, false
		}
		return "game_merging_selection", data, true
	case domain.RoomStatusMergingSettle:
		settle := chooseMergingSettleForAI(r, playerID)
		data, err := json.Marshal(map[string]interface{}{"actions": settle})
		if err != nil {
			return "", nil, false
		}
		return "game_merging_settle", data, true
	default:
		return "", nil, false
	}
}

// BuildTurnTimeoutCommand 思考超时时由 roomcore 调用。
// 与 MaybeRunAIIfNeeded 共用 buildAIActionMsg，但 PlayerID 用真实玩家 ID，且不修改身份。
func BuildTurnTimeoutCommand(r *domain.Room, playerID string) (roomcore.Command, bool) {
	cmdType, payload, ok := buildAIActionMsg(r, playerID, r.State.RoomStatus, nil)
	if !ok {
		return roomcore.Command{}, false
	}
	return roomcore.Command{
		Type:     cmdType,
		PlayerID: playerID,
		Payload:  payload,
		Conn:     &VirtualConn{Room: r},
	}, true
}

func MaybeRunAIIfNeeded(r *domain.Room, message []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		logger.Error("AI 消息格式错误", logger.F("room_id", r.ID), logger.F("error", err))
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
	gameStatusStr, ok := roomData["gameStatus"].(string)
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
		}

		if !isAI {
			logger.Info("当前玩家 %s 不是 AI 玩家", logger.F("player_id", currentPlayerID))
			return false
		}
	}

	// 检查是否已经有 AI 行动在运行，防止多个 AI 玩家同时触发
	if r.AIRunning {
		logger.Info("AI 行动已在运行中，跳过重复触发", logger.F("room_id", r.ID), logger.F("player_id", playerId))
		return false
	}

	// 提取临时数据（合并选择）
	tempData, ok := msg["tempData"].(map[string]interface{})
	if !ok {
		logger.Error("tempData 类型错误", logger.F("room_id", r.ID))
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
		logger.Error("所有 tile 已被使用", logger.F("room_id", r.ID), logger.F("player_id", playerId))
	}

	// 标记 AI 行动已开始
	r.AIRunning = true
	logger.Info("当前是 AI 玩家的回合，准备延迟执行 AI 行动", logger.F("room_id", r.ID), logger.F("player_id", playerId), logger.F("game_status", gameStatus))

	// ---------- 在协程中延迟执行 ----------
	go func() {
		defer func() {
			// 无论如何都要重置标志
			r.AIRunning = false
		}()
		time.Sleep(5 * time.Second)

		cmdType, payload, ok := buildAIActionMsg(r, playerId, gameStatus, mainCompany)
		if !ok {
			logger.Warn("AI 未生成有效动作", logger.F("room_id", r.ID), logger.F("player_id", playerId), logger.F("game_status", gameStatus))
			return
		}
		logger.Info("AI 发送消息", logger.F("room_id", r.ID), logger.F("player_id", playerId), logger.F("type", cmdType), logger.F("payload", string(payload)))

		// 向房间的命令通道发送消息，和玩家一样的处理方式
		r.CmdCh <- domain.Command{
			Type:     cmdType,
			PlayerID: playerId,
			Payload:  payload,
			Conn:     &VirtualConn{Room: r},
		}
	}()

	return true
}
