package roompkg

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/utils"
	"github.com/nciyuan9264/game-backend/pkg/arrayutil"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"

	"math/rand/v2"
)

const (
	aiSafeCompanyTiles = 11 // 与 step1.go 一致：tile 数 >= 11 视为 safe（不可被吞）
	aiMaxStockPerCo    = 13 // 单家公司持股上限
	aiMaxStockPerTurn  = 3  // 每回合最多购股数
	aiFounderBonus     = 300
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

// ----------------------------------------------------------------------------
// AI 评估器：只读 helper，不修改 r.State
// ----------------------------------------------------------------------------

// simulateTilePlacement 模拟把 tileKey 放到棋盘上后，会"邻接"哪些公司、Blank 块。
// 不修改 BoardTiles。illegal=true 表示该落子会同时连接 ≥2 家 safe 公司（≥11 tile），
// 这种落子在游戏侧只是空转，AI 应当回避。
func simulateTilePlacement(r *domain.Room, tileKey string) (companies map[string]struct{}, blankCount int, illegal bool) {
	companies = make(map[string]struct{})
	for _, adj := range data.GetAdjacentTileKeys(tileKey) {
		t, ok := r.State.BoardTiles[adj]
		if !ok || t == nil {
			continue
		}
		switch t.Belong {
		case "":
			continue
		case "Blank":
			blankCount++
		default:
			companies[t.Belong] = struct{}{}
		}
	}
	if len(companies) >= 2 {
		safeCount := 0
		for c := range companies {
			if info, ok := r.State.Companies[c]; ok && info.Tiles >= aiSafeCompanyTiles {
				safeCount++
			}
		}
		if safeCount >= 2 {
			illegal = true
		}
	}
	return
}

// holdersRanking 返回某公司持股从高到低的 (playerID, count) 排序。
func holdersRanking(r *domain.Room, company string) []struct {
	PlayerID string
	Count    int
} {
	type entry = struct {
		PlayerID string
		Count    int
	}
	out := make([]entry, 0, len(r.State.Players))
	for pid, p := range r.State.Players {
		if p == nil {
			continue
		}
		if c := p.Stocks[company]; c > 0 {
			out = append(out, entry{PlayerID: pid, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// dominanceScore 描述 me 在 company 中的"主导地位"：
//
//	2 = 第一大股东（含并列），1 = 第二大股东（含并列），0 = 暂无人持股，-1 = 落后
func dominanceScore(r *domain.Room, me string, company string) int {
	rank := holdersRanking(r, company)
	if len(rank) == 0 {
		return 0
	}
	myCount := 0
	if p, ok := r.State.Players[me]; ok && p != nil {
		myCount = p.Stocks[company]
	}
	if myCount == 0 {
		return -1
	}
	first := rank[0].Count
	if myCount >= first {
		return 2
	}
	// second
	second := 0
	for _, e := range rank {
		if e.Count < first {
			second = e.Count
			break
		}
	}
	if myCount >= second && second > 0 {
		return 1
	}
	return -1
}

// stockInfoOf 安全地按公司名 + tile 数取价格档位信息。
func stockInfoOf(company string, tileCount int) *utils.StockInfo {
	t, ok := utils.ParseCompanyType(company)
	if !ok {
		return nil
	}
	return utils.GetStockInfo(t, tileCount)
}

// bandStepGain 估算公司 tile 数增加 delta 后，价格 / 第一大奖金 的跨档涨幅。
func bandStepGain(company string, currentTiles, delta int) (priceDelta int, bonusFirstDelta int) {
	cur := stockInfoOf(company, currentTiles)
	nxt := stockInfoOf(company, currentTiles+delta)
	if cur == nil || nxt == nil {
		return 0, 0
	}
	return nxt.Price - cur.Price, nxt.BonusFirst - cur.BonusFirst
}

// ----------------------------------------------------------------------------
// 决策函数
// ----------------------------------------------------------------------------

// scoreTilePlacement 给一张候选 tile 评分，分数越高越值得下。
func scoreTilePlacement(r *domain.Room, me, tileKey string, companies map[string]struct{}, blankCount int) int {
	if len(companies) == 0 && blankCount == 0 {
		return 1 // 孤立落点，保底分
	}

	if len(companies) == 1 {
		var only string
		for c := range companies {
			only = c
		}
		info := r.State.Companies[only]
		dom := dominanceScore(r, me, only)
		// 扩张后该公司新增 tile 数 ≈ blankCount + 1（含放下的这块）
		_, bonusGain := bandStepGain(only, info.Tiles, blankCount+1)
		score := dom*200 + bonusGain/10
		if dom <= 0 {
			// 给对手扩张是负价值
			score -= info.StockPrice
		} else {
			score += info.StockPrice
		}
		return score
	}

	if len(companies) >= 2 {
		// 触发并购：累计奖金期望与对存活方持股价值
		score := 0
		// 选 tile 最多的为存活方近似
		mainTiles := -1
		var main string
		for c := range companies {
			if t := r.State.Companies[c].Tiles; t > mainTiles {
				mainTiles = t
				main = c
			}
		}
		// 被吞公司的奖金期望
		for c := range companies {
			if c == main {
				continue
			}
			info := r.State.Companies[c]
			si := stockInfoOf(c, info.Tiles)
			if si == nil {
				continue
			}
			dom := dominanceScore(r, me, c)
			switch dom {
			case 2:
				score += si.BonusFirst / 10
			case 1:
				score += si.BonusSecond / 10
			default:
				// 不持股：合并对自己中性
			}
			// 把被吞公司的持股按当前价折现
			if p, ok := r.State.Players[me]; ok && p != nil {
				score += p.Stocks[c] * info.StockPrice / 10
			}
		}
		// 对存活方的持股估值
		if main != "" {
			info := r.State.Companies[main]
			if p, ok := r.State.Players[me]; ok && p != nil {
				score += p.Stocks[main] * info.StockPrice / 10
			}
		}
		return score
	}

	// 仅连接 Blank：可能触发"创建公司"。
	if blankCount >= 1 {
		hasUncreated := false
		for _, info := range r.State.Companies {
			if info.Tiles == 0 {
				hasUncreated = true
				break
			}
		}
		if hasUncreated {
			// 估算创建后的初始体量（连同 tileKey 自身的 Blank 连通块）
			predictedSize := blankCount + 1
			score := aiFounderBonus + predictedSize*50
			return score
		}
	}
	return 1
}

func chooseTileForAI(room *domain.Room, playerID string) string {
	tiles := room.State.Players[playerID].Tiles
	if len(tiles) == 0 {
		return ""
	}

	bestScore := math.MinInt
	best := ""
	legalFallback := ""
	for _, t := range tiles {
		companies, blanks, illegal := simulateTilePlacement(room, t)
		if illegal {
			continue
		}
		if legalFallback == "" {
			legalFallback = t
		}
		s := scoreTilePlacement(room, playerID, t, companies, blanks)
		if s > bestScore {
			bestScore = s
			best = t
		}
	}
	if best != "" {
		return best
	}
	if legalFallback != "" {
		return legalFallback
	}
	// 全部 tile 都会触发违规并购：不得已随机
	return tiles[rand.IntN(len(tiles))]
}

func chooseCompanyForAI(r *domain.Room) string {
	var uncreated []string
	for company, info := range r.State.Companies {
		if info.Tiles == 0 {
			uncreated = append(uncreated, company)
		}
	}
	if len(uncreated) == 0 {
		return ""
	}

	priority1 := []string{"Continental", "Imperial"}
	priority2 := []string{"American", "Festival", "Worldwide"}
	var p1, p2, p3 []string
	for _, c := range uncreated {
		switch {
		case arrayutil.StringInSlice(c, priority1):
			p1 = append(p1, c)
		case arrayutil.StringInSlice(c, priority2):
			p2 = append(p2, c)
		default:
			p3 = append(p3, c)
		}
	}

	// 当 LastTileKey 所在的 Blank 连通块较大时，优先选 Premium 公司（奖金跨档更大）
	predictedSize := 0
	if r.State.LastTileKey != "" {
		predictedSize = len(data.GetConnectedTiles(r, r.State.LastTileKey))
	}
	if predictedSize >= 6 && len(p1) > 0 {
		return p1[0]
	}

	if len(p1) > 0 {
		return p1[0]
	}
	if len(p2) > 0 {
		return p2[0]
	}
	return p3[0]
}

// scoreBuyCandidate 为购股候选打 ROI 分。
func scoreBuyCandidate(r *domain.Room, me, company string) int {
	info := r.State.Companies[company]
	si := stockInfoOf(company, info.Tiles)
	if si == nil {
		return math.MinInt
	}
	dom := dominanceScore(r, me, company)
	// 进档时 1st 奖金跨档收益（粗略估计）
	_, bonusGain := bandStepGain(company, info.Tiles, 1)
	roi := bonusGain / 5
	roi += dom * info.StockPrice
	roi -= info.StockPrice
	// safe 公司加成（稳定收益）
	if info.Tiles >= aiSafeCompanyTiles {
		roi += info.StockPrice / 2
	}
	return roi
}

func chooseStocksToBuyForAI(r *domain.Room, playerID string) map[string]int {
	playerInfo := r.State.Players[playerID]
	money := playerInfo.Money
	playerStock := playerInfo.Stocks

	// 用临时持股快照在每次买入后递增，使 dominanceScore 在循环内反映买后状态
	tmpStock := make(map[string]int, len(playerStock))
	for k, v := range playerStock {
		tmpStock[k] = v
	}

	result := map[string]int{}
	stockCount := 0
	for stockCount < aiMaxStockPerTurn {
		// 候选：已创建、有库存、未达上限、买得起
		type candidate struct {
			Name  string
			Score int
			Price int
		}
		var options []candidate
		// 把临时持股写回再评估
		original := r.State.Players[playerID].Stocks
		r.State.Players[playerID].Stocks = tmpStock
		for name, info := range r.State.Companies {
			remain := info.StockTotal - result[name]
			if info.Tiles == 0 || remain <= 0 || tmpStock[name] >= aiMaxStockPerCo || info.StockPrice > money {
				continue
			}
			s := scoreBuyCandidate(r, playerID, name)
			options = append(options, candidate{Name: name, Score: s, Price: info.StockPrice})
		}
		r.State.Players[playerID].Stocks = original

		if len(options) == 0 {
			break
		}
		sort.Slice(options, func(i, j int) bool {
			if options[i].Score != options[j].Score {
				return options[i].Score > options[j].Score
			}
			return options[i].Price > options[j].Price // 同分时优先贵的（潜力更大）
		})
		best := options[0]
		if best.Score <= 0 {
			break
		}
		result[best.Name]++
		tmpStock[best.Name]++
		money -= best.Price
		stockCount++
		if money <= 0 {
			break
		}
	}

	return result
}

func chooseMergingSettleForAI(r *domain.Room, playerID string) []domain.MergingSettleItem {
	result := []domain.MergingSettleItem{}
	mainCompanyInfo := r.State.Companies[r.State.MergeMainCompany]

	for companyKey := range r.State.MergeSettleData {
		count := r.State.Players[playerID].Stocks[companyKey]
		if count == 0 {
			continue
		}
		company := r.State.Companies[companyKey]

		exchangeAmount := 0
		sellAmount := count

		// 2 股换 1 股：mainPrice >= 2*absorbedPrice 时换股净收益 ≥ 0；
		// 主导地位 ≥ 1（已是 1st/2nd）则放宽到 mainPrice >= absorbedPrice。
		mainDom := dominanceScore(r, playerID, r.State.MergeMainCompany)
		shouldExchange := mainCompanyInfo.StockPrice >= 2*company.StockPrice
		if !shouldExchange && mainDom >= 1 && mainCompanyInfo.StockPrice >= company.StockPrice {
			shouldExchange = true
		}

		if shouldExchange {
			maxEven := count
			if maxEven%2 != 0 {
				maxEven -= 1
			}
			// 主公司 stockTotal 限制：每 2 股换 1 股
			maxCanExchange := mainCompanyInfo.StockTotal * 2
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

// AutoSettleDisconnectedHolder 在玩家断线且处于并购结算阶段时，
// 自动替该离线持股玩家提交一次结算，避免结算队列因其离线而永久卡死。
func AutoSettleDisconnectedHolder(r *domain.Room, playerID string) {
	if r == nil || r.State == nil || r.State.RoomStatus != domain.RoomStatusMergingSettle {
		return
	}

	inHolder := false
	for _, d := range r.State.MergeSettleData {
		for _, h := range d.Hoders {
			if h == playerID {
				inHolder = true
				break
			}
		}
		if inHolder {
			break
		}
	}
	if !inHolder {
		return
	}

	actions := chooseMergingSettleForAI(r, playerID)
	payload, err := json.Marshal(map[string]interface{}{"actions": actions})
	if err != nil {
		logger.Error("自动结算编码失败", logger.F("room_id", r.ID), logger.F("player_id", playerID), logger.F("error", err))
		return
	}

	logger.Info("玩家离线，自动替其完成并购结算", logger.F("room_id", r.ID), logger.F("player_id", playerID))
	game.HandleMergingSettleMessage(r, domain.Command{
		Type:     "game_merging_settle",
		PlayerID: playerID,
		Payload:  payload,
	})
}

func chooseMergingSelectionForAI(r *domain.Room, playerID string, mainCompany []string) string {
	if len(mainCompany) == 0 {
		return ""
	}

	// 估算合并后存活方的 tile 总和（用候选 + 其他 main 候选 tile 之和近似，
	// 因为最终被吞公司中只有 ≥11 tile 的会被剔除，副公司大部分会并入存活方）。
	otherTilesSum := 0
	for _, c := range mainCompany {
		otherTilesSum += r.State.Companies[c].Tiles
	}

	res := ""
	bestGain := math.MinInt
	for _, companyKey := range mainCompany {
		info := r.State.Companies[companyKey]
		// 合并后 tile 数 ≈ otherTilesSum（自身 + 其他副公司之和）
		predicted := otherTilesSum
		si := stockInfoOf(companyKey, predicted)
		if si == nil {
			continue
		}
		myStocks := r.State.Players[playerID].Stocks[companyKey]
		gain := myStocks * si.Price
		dom := dominanceScore(r, playerID, companyKey)
		switch dom {
		case 2:
			gain += si.BonusFirst
		case 1:
			gain += si.BonusSecond
		}
		// 平衡因素：不要选已 safe 但我没持股的
		if myStocks == 0 && info.Tiles >= aiSafeCompanyTiles {
			gain -= si.Price * 2
		}
		if gain > bestGain {
			bestGain = gain
			res = companyKey
		}
	}
	if res == "" {
		// 兜底：取数组首项
		return mainCompany[0]
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
			// 兜底：从已创建公司里挑一个 Tiles>0 的，保证能继续推进。
			selection = anyActiveCompany(r)
		}
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
	case domain.RoomStatusMerging:
		// merging 是过渡态，正常会被 handler 立刻切走；落到此处说明状态机异常，交给系统级兜底。
		return "", nil, false
	default:
		return "", nil, false
	}
}

// anyActiveCompany 兜底：从已创建（Tiles>0）的公司里取一个名字（按字典序）。
func anyActiveCompany(r *domain.Room) string {
	if r == nil || r.State == nil || r.State.Companies == nil {
		return ""
	}
	names := make([]string, 0, len(r.State.Companies))
	for name, info := range r.State.Companies {
		if info != nil && info.Tiles > 0 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

// BuildTurnTimeoutCommand 思考超时时由 roomcore 调用。
// 与 MaybeRunAIIfNeeded 共用 buildAIActionMsg，但 PlayerID 用真实玩家 ID，且不修改身份。
func BuildTurnTimeoutCommand(r *domain.Room, playerID string) (roomcore.Command, bool) {
	cmdType, payload, ok := buildAIActionMsg(r, playerID, r.State.RoomStatus, nil)
	if !ok {
		// 兜底：发出 turn_timeout 强制切人，避免房间永久卡死。
		return roomcore.Command{
			Type:     "turn_timeout",
			PlayerID: playerID,
			Payload:  []byte("{}"),
			Conn:     &VirtualConn{Room: r},
		}, true
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
			logger.Warn("AI 未生成有效动作，发送 turn_timeout 兜底",
				logger.F("room_id", r.ID),
				logger.F("player_id", playerId),
				logger.F("game_status", gameStatus))
			r.CmdCh <- domain.Command{
				Type:     "turn_timeout",
				PlayerID: playerId,
				Payload:  []byte("{}"),
				Conn:     &VirtualConn{Room: r},
			}
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
