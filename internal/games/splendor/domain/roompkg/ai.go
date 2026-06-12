package roompkg

import (
	"encoding/json"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	spgame "github.com/nciyuan9264/game-backend/internal/games/splendor/domain/game"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// aiGemColors 是 AI 可拿取的五种宝石颜色，固定顺序保证决策确定性（与 map 迭代无序解耦）。
var aiGemColors = []string{"Black", "Blue", "Green", "Red", "White"}

// aiMaxTokens 与 handler 中 maxTokensInHand 保持一致：玩家手中宝石（含 Gold）上限 10。
const aiMaxTokens = 10

// VirtualConn 用作 AI 玩家的写连接占位。它不连接真实客户端，
// 只是在 BroadcastToRoom 给 AI 投递 sync 消息时被调用，从而触发 AI 决策。
type VirtualConn struct {
	Room *domain.Room
}

// 编译期断言实现 WriteOnlyConn
var _ domain.WriteOnlyConn = (*VirtualConn)(nil)

func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	MaybeRunAIIfNeeded(v.Room, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, nil
}

func (v *VirtualConn) Close() error {
	return nil
}

// IsAIPlayerID 判断 playerID 是否为 AI 玩家（基于命名约定）。
func IsAIPlayerID(playerID string) bool {
	return strings.HasPrefix(playerID, "ai_")
}

// candidateAction 表示 AI 本回合一个候选合法动作及其命令编码。
type candidateAction struct {
	cmdType string
	payload []byte
	// applyKind / 参数用于在模拟时直接调用对应 handler。
	desc string
}

// buildAIActionMsg 用「单步前瞻 + 评估函数」选出本回合最优动作。
//
// 流程：枚举本回合所有合法动作 → 在 GameState 深拷贝上复用真实 handler 模拟到结果态
// → 用确定性评估函数 evaluateState 给"行动玩家结果态"打分 → 取最高分动作。
// 这样能统一处理买卡/拿宝石/保留卡，并自然涵盖贵族进度、资源效率与终局冲刺。
// 返回 ok=false 时调用方走 turn_timeout 兜底切人，避免房间死锁。
func buildAIActionMsg(r *domain.Room, playerID string, status domain.RoomStatus) (cmdType string, payload []byte, ok bool) {
	if status != domain.RoomStatusPlaying && status != domain.RoomStatusLastTurn {
		return "", nil, false
	}
	if aiPlayerState(r, playerID) == nil {
		return "", nil, false
	}

	cands := enumerateActions(r, playerID)
	if len(cands) == 0 {
		return "", nil, false
	}

	bestScore := -1 << 62
	var best *candidateAction
	for i := range cands {
		c := &cands[i]
		score, okSim := simulateAndEvaluate(r, playerID, c)
		if !okSim {
			continue
		}
		if best == nil || score > bestScore || (score == bestScore && c.desc < best.desc) {
			bestScore = score
			best = c
		}
	}
	if best == nil {
		return "", nil, false
	}
	return best.cmdType, best.payload, true
}

// enumerateActions 枚举当前 AI 本回合所有合法动作（与 handler 校验对齐）。
func enumerateActions(r *domain.Room, playerID string) []candidateAction {
	ps := aiPlayerState(r, playerID)
	if ps == nil {
		return nil
	}
	out := make([]candidateAction, 0, 64)

	// 1) 买卡：桌面明牌 + 自己保留卡中买得起的。
	bonus := cardBonusCount(ps)
	for _, c := range collectBuyableCards(r, ps) {
		if !affordableCard(ps, c, bonus) {
			continue
		}
		if data, err := json.Marshal(c.ID); err == nil {
			out = append(out, candidateAction{
				cmdType: "game_buy_card",
				payload: data,
				desc:    "buy_" + itoa(c.ID),
			})
		}
	}

	// 2) 拿宝石：所有合法组合（≤3 异色各 1 / 单色 2 且库存≥4），拿后手牌≤10。
	for _, gems := range enumerateGemTakes(r, ps) {
		if data, err := json.Marshal(gems); err == nil {
			out = append(out, candidateAction{
				cmdType: "game_get_gem",
				payload: data,
				desc:    "gem_" + gemsDesc(gems),
			})
		}
	}

	// 3) 保留卡：保留区未满时，可保留任一桌面明牌。
	if len(ps.ReserveCard) < 3 {
		for _, c := range revealedBoardCards(r) {
			if data, err := json.Marshal(c.ID); err == nil {
				out = append(out, candidateAction{
					cmdType: "game_preserve_card",
					payload: data,
					desc:    "reserve_" + itoa(c.ID),
				})
			}
		}
	}

	return out
}

// enumerateGemTakes 枚举所有合法的拿宝石组合（颜色→数量）。
func enumerateGemTakes(r *domain.Room, ps *domain.PlayerState) []map[string]int {
	canTake := aiMaxTokens - aiCountTokens(ps)
	if canTake <= 0 {
		return nil
	}

	// 可拿的颜色（库存>0），固定顺序保证确定性。
	avail := make([]string, 0, len(aiGemColors))
	for _, color := range aiGemColors {
		if r.State.Gems[color] > 0 {
			avail = append(avail, color)
		}
	}

	out := make([]map[string]int, 0, 16)

	// (a) 同色 2 个：库存≥4 且 canTake≥2。
	if canTake >= 2 {
		for _, color := range aiGemColors {
			if r.State.Gems[color] >= 4 {
				out = append(out, map[string]int{color: 2})
			}
		}
	}

	// (b) 不同色各 1 个：取 1..min(3, canTake, len(avail)) 种的所有组合。
	maxPick := 3
	if canTake < maxPick {
		maxPick = canTake
	}
	if len(avail) < maxPick {
		maxPick = len(avail)
	}
	for k := 1; k <= maxPick; k++ {
		combos := combinations(avail, k)
		for _, combo := range combos {
			m := make(map[string]int, k)
			for _, color := range combo {
				m[color] = 1
			}
			out = append(out, m)
		}
	}
	return out
}

// simulateAndEvaluate 在 GameState 深拷贝上应用候选动作（复用真实 handler），
// 返回行动玩家在结果态的评估分。共享 Base（handler 只读 PlayerSeq/Connections，不修改）。
func simulateAndEvaluate(r *domain.Room, playerID string, c *candidateAction) (int, bool) {
	clonedState := cloneGameState(r.State)
	if clonedState == nil {
		return 0, false
	}
	sim := &domain.Room{Base: r.Base, State: clonedState}
	cmd := domain.Command{Type: c.cmdType, PlayerID: playerID, Payload: c.payload}

	switch c.cmdType {
	case "game_buy_card":
		spgame.HandleBuyCardMessage(sim, cmd)
	case "game_get_gem":
		spgame.HandleGetGemMessage(sim, cmd)
	case "game_preserve_card":
		spgame.HandleReserveCardMessage(sim, cmd)
	default:
		return 0, false
	}

	// 校验动作确实被执行了（handler 拒绝时不会切走 CurrentPlayer）。
	if clonedState.CurrentPlayer == playerID {
		return 0, false
	}
	return evaluateState(clonedState, playerID), true
}

// evaluateState 评估"行动玩家"在给定结果态的局面优劣（越大越好），完全确定、可复现。
//
// 维度：
//   - 荣誉分（核心目标，权重最高）；
//   - 发展卡折扣资产（永久收益）；
//   - 贵族进度（向最接近的一位贵族的折扣缺口收敛）；
//   - 买卡进度（向桌面性价比最高的目标卡收敛，鼓励有效囤资源）；
//   - 宝石/Gold 轻微正权重（流动性），但对超过上限的冗余不额外奖励；
//   - 终局冲刺：进入 last_turn 或自身≥15 分时，荣誉分权重进一步放大。
func evaluateState(s *domain.GameState, playerID string) int {
	ps := s.Players[playerID]
	if ps == nil {
		return -1 << 60
	}

	bonus := make(map[string]int)
	for _, c := range ps.NormalCard {
		bonus[c.Bonus]++
	}

	score := 0

	// 荣誉分：基础权重 1000；终局阶段放大到 2000。
	scoreWeight := 1000
	if s.RoomStatus == domain.RoomStatusLastTurn || s.RoomStatus == domain.RoomStatusEnd || ps.Score >= 15 {
		scoreWeight = 2000
	}
	score += ps.Score * scoreWeight

	// 发展卡折扣资产：每张永久折扣值 60。
	score += len(ps.NormalCard) * 60

	// 贵族进度：取最接近的一位明示贵族，缺口越小加分越多。
	score += nobleProgress(s, bonus) * 25

	// 买卡进度：朝桌面"性价比最高目标卡"的剩余缺口收敛（缺口越小越好）。
	score -= bestCardRemainingCost(s, ps, bonus) * 8

	// 宝石流动性：每个宝石 3 分、Gold 5 分（万能更值钱）。
	for color, n := range ps.Gem {
		if color == "Gold" {
			score += n * 5
		} else {
			score += n * 3
		}
	}

	return score
}

// nobleProgress 返回"距离最近一位明示贵族"的进度分：已满足的折扣需求数（取最优贵族）。
func nobleProgress(s *domain.GameState, bonus map[string]int) int {
	best := 0
	for _, n := range s.NobleCards {
		if n == nil || n.State != entities.CardStateRevealed {
			continue
		}
		matched, total := 0, 0
		for color, req := range n.Cost {
			total += req
			matched += min(bonus[color], req)
		}
		// 进度 = 已满足/总需求 的百分比（0..100），取最高的一位贵族。
		if total > 0 {
			p := matched * 100 / total
			if p > best {
				best = p
			}
		}
	}
	return best
}

// bestCardRemainingCost 返回"桌面性价比最高目标卡"在折扣后仍缺的宝石数（含需用 Gold 补的）。
// 性价比 = 荣誉分优先、其次折扣色在桌面需求中的总量；缺口越小代表越接近买下它。
func bestCardRemainingCost(s *domain.GameState, ps *domain.PlayerState, bonus map[string]int) int {
	type cand struct {
		id        int
		quality   int
		remaining int
	}
	var picks []cand
	for _, c := range s.NormalCards {
		if c == nil || c.State != entities.CardStateRevealed {
			continue
		}
		remaining := 0
		for color, cost := range c.Cost {
			need := cost - bonus[color] - ps.Gem[color]
			if need > 0 {
				remaining += need
			}
		}
		quality := c.Points*10 + cardColorDemand(s, c.Bonus)
		picks = append(picks, cand{id: c.ID, quality: quality, remaining: remaining})
	}
	if len(picks) == 0 {
		return 0
	}
	sort.Slice(picks, func(i, j int) bool {
		if picks[i].quality != picks[j].quality {
			return picks[i].quality > picks[j].quality
		}
		return picks[i].id < picks[j].id
	})
	return picks[0].remaining
}

// cardColorDemand 统计某折扣色在桌面所有明牌 Cost 中的需求总量。
func cardColorDemand(s *domain.GameState, color string) int {
	total := 0
	for _, c := range s.NormalCards {
		if c != nil && c.State == entities.CardStateRevealed {
			total += c.Cost[color]
		}
	}
	return total
}

// cloneGameState 通过 JSON 往返深拷贝 GameState，切断与真实状态的指针共享，供模拟使用。
func cloneGameState(s *domain.GameState) *domain.GameState {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	out := &domain.GameState{}
	if err := json.Unmarshal(b, out); err != nil {
		return nil
	}
	return out
}

// combinations 返回 items 中取 k 个的所有组合（保持输入顺序，确定性）。
func combinations(items []string, k int) [][]string {
	var res [][]string
	var helper func(start int, cur []string)
	helper = func(start int, cur []string) {
		if len(cur) == k {
			cp := make([]string, k)
			copy(cp, cur)
			res = append(res, cp)
			return
		}
		for i := start; i < len(items); i++ {
			helper(i+1, append(cur, items[i]))
		}
	}
	helper(0, make([]string, 0, k))
	return res
}

// gemsDesc 把宝石组合编码为稳定字符串（用于平局时的确定性排序）。
func gemsDesc(gems map[string]int) string {
	var b strings.Builder
	for _, color := range aiGemColors {
		if n := gems[color]; n > 0 {
			b.WriteString(color)
			b.WriteByte(byte('0' + n))
		}
	}
	return b.String()
}

// itoa 简单整数转字符串（避免引入 strconv 仅为一处使用）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// aiPlayerState 取当前 AI 玩家的局内状态。
func aiPlayerState(r *domain.Room, playerID string) *domain.PlayerState {
	if r == nil || r.State == nil || r.State.Players == nil {
		return nil
	}
	return r.State.Players[playerID]
}

// cardBonusCount 统计玩家已拥有的各色发展卡数量（用于买卡折扣）。
func cardBonusCount(ps *domain.PlayerState) map[string]int {
	m := make(map[string]int)
	for _, c := range ps.NormalCard {
		m[c.Bonus]++
	}
	return m
}

// aiCountTokens 统计玩家手中宝石总数（含 Gold）。
func aiCountTokens(ps *domain.PlayerState) int {
	total := 0
	for _, n := range ps.Gem {
		total += n
	}
	return total
}

// revealedBoardCards 返回桌面所有明牌，按 ID 升序（确定性）。
func revealedBoardCards(r *domain.Room) []*entities.NormalCard {
	cards := make([]*entities.NormalCard, 0, len(r.State.NormalCards))
	for _, c := range r.State.NormalCards {
		if c != nil && c.State == entities.CardStateRevealed {
			cards = append(cards, c)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}

// collectBuyableCards 收集 AI 可购买的候选卡：桌面明牌 + 自己的保留卡，按 ID 去重并升序。
func collectBuyableCards(r *domain.Room, ps *domain.PlayerState) []*entities.NormalCard {
	seen := make(map[int]bool)
	cards := make([]*entities.NormalCard, 0, len(r.State.NormalCards)+len(ps.ReserveCard))
	for _, c := range revealedBoardCards(r) {
		if !seen[c.ID] {
			seen[c.ID] = true
			cards = append(cards, c)
		}
	}
	for i := range ps.ReserveCard {
		c := &ps.ReserveCard[i]
		if !seen[c.ID] {
			seen[c.ID] = true
			cards = append(cards, c)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}

// affordableCard 复刻 handler 支付模型，判断玩家能否买得起该卡：
// 每色按 cost-该色发展卡折扣后，先用对应色宝石，不足用 Gold 补；任一色补不上则买不起。
func affordableCard(ps *domain.PlayerState, card *entities.NormalCard, bonus map[string]int) bool {
	goldLeft := ps.Gem["Gold"]
	for color, cost := range card.Cost {
		need := cost - bonus[color]
		if need <= 0 {
			continue
		}
		if ps.Gem[color] >= need {
			continue
		}
		goldLeft -= need - ps.Gem[color]
		if goldLeft < 0 {
			return false
		}
	}
	return true
}

// BuildTurnTimeoutCommand 思考超时时由 roomcore 调用：
// 若 AI 能给出有效动作则投递该动作，否则发 turn_timeout 强制切人，避免房间永久卡死。
func BuildTurnTimeoutCommand(r *domain.Room, playerID string) (roomcore.Command, bool) {
	cmdType, payload, ok := buildAIActionMsg(r, playerID, r.State.RoomStatus)
	if !ok {
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

// MaybeRunAIIfNeeded 在 AI 玩家收到 sync 消息时，按需在新 goroutine 里给房间 CmdCh 投递命令。
// 当前 splendor 无 AI 决策，统一投递 turn_timeout 兜底切人，确保有 AI 在场时游戏不会卡死。
func MaybeRunAIIfNeeded(r *domain.Room, data []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("AI 消息格式错误 room=%s err=%v", r.ID, err)
		return false
	}

	roomData, ok := msg["roomData"].(map[string]interface{})
	if !ok {
		return false
	}
	currentPlayerID, ok := roomData["currentPlayer"].(string)
	if !ok || currentPlayerID == "" {
		return false
	}

	roomInfo, ok := roomData["roomInfo"].(map[string]interface{})
	if !ok {
		return false
	}
	gameStatusStr, ok := roomInfo["gameStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := domain.RoomStatus(gameStatusStr)
	if gameStatus == domain.RoomStatusEnd {
		return false
	}

	if !IsAIPlayerID(currentPlayerID) {
		return false
	}

	if r.AIRunning {
		return false
	}
	r.AIRunning = true

	go func() {
		defer func() { r.AIRunning = false }()
		// 模拟思考：随机 5~8 秒，让 AI 出手节奏更自然、不过快。
		time.Sleep(time.Duration(5000+rand.Intn(3001)) * time.Millisecond)

		cmdType, payload, ok := buildAIActionMsg(r, currentPlayerID, gameStatus)
		if !ok {
			// 无有效动作：发 turn_timeout 兜底切人。
			r.CmdCh <- domain.Command{
				Type:     "turn_timeout",
				PlayerID: currentPlayerID,
				Payload:  []byte("{}"),
				Conn:     &VirtualConn{Room: r},
			}
			return
		}

		r.CmdCh <- domain.Command{
			Type:     cmdType,
			PlayerID: currentPlayerID,
			Payload:  payload,
			Conn:     &VirtualConn{Room: r},
		}
	}()

	return true
}
