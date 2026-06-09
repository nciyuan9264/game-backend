package roompkg

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
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
	return nil
}

// maxEnumerationStates 限制单个对手暗牌枚举的状态数，避免极端局面阻塞房间循环。
const maxEnumerationStates = 200000

// cardOrderLess 数字升序、同数字黑牌在白牌前。
func cardOrderLess(numA domain.CardNumber, colorA domain.Color, numB domain.CardNumber, colorB domain.Color) bool {
	if numA != numB {
		return numA < numB
	}
	return colorA == domain.ColorBlack && colorB == domain.ColorWhite
}

// buildKnownNumbers AI 视角下已知不再可能出现的数字（按颜色分桶）。
// 已知来源仅包含：AI 自己手上的所有牌（含未放置的新牌） + 所有玩家已翻开的牌。
// 不能使用服务端隐藏的对手暗牌真实数字或公共牌真实数字。
func buildKnownNumbers(r *domain.Room, playerID string) map[domain.Color]map[domain.CardNumber]bool {
	known := map[domain.Color]map[domain.CardNumber]bool{
		domain.ColorWhite: {},
		domain.ColorBlack: {},
	}
	if r == nil || r.State == nil || r.State.Players == nil {
		return known
	}
	for pid, ps := range r.State.Players {
		if ps == nil {
			continue
		}
		for _, c := range ps.Cards {
			if c == nil {
				continue
			}
			if pid == playerID || c.IsRevealed {
				known[c.Color][c.Num] = true
			}
		}
	}
	return known
}

// enumerateOwnerDistribution 对一名对手的暗牌做一致性回溯，
// 返回每张暗牌每个数字的命中状态计数和总状态数。
// 约束：颜色固定；同色数字唯一；按 Index 升序排列后整体序列严格满足 cardOrderLess；
// 已被 known 占用的数字不可再分配。
func enumerateOwnerDistribution(ps *domain.PlayerState, known map[domain.Color]map[domain.CardNumber]bool) (map[string]map[domain.CardNumber]int, int) {
	if ps == nil {
		return nil, 0
	}
	cards := make([]*domain.Card, 0, len(ps.Cards))
	for _, c := range ps.Cards {
		if c != nil {
			cards = append(cards, c)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Index < cards[j].Index })

	usedWhite := map[domain.CardNumber]bool{}
	usedBlack := map[domain.CardNumber]bool{}
	for n := range known[domain.ColorWhite] {
		usedWhite[n] = true
	}
	for n := range known[domain.ColorBlack] {
		usedBlack[n] = true
	}

	counts := map[string]map[domain.CardNumber]int{}
	for _, c := range cards {
		if !c.IsRevealed {
			counts[c.ID] = map[domain.CardNumber]int{}
		}
	}
	if len(counts) == 0 {
		return counts, 0
	}

	chosen := make([]domain.CardNumber, len(cards))
	total := 0
	stopped := false

	var backtrack func(i int)
	backtrack = func(i int) {
		if stopped {
			return
		}
		if i == len(cards) {
			total++
			for j, c := range cards {
				if c.IsRevealed {
					continue
				}
				counts[c.ID][chosen[j]]++
			}
			if total >= maxEnumerationStates {
				stopped = true
			}
			return
		}
		c := cards[i]
		if c.IsRevealed {
			if i > 0 {
				prev := cards[i-1]
				if !cardOrderLess(chosen[i-1], prev.Color, c.Num, c.Color) {
					return
				}
			}
			chosen[i] = c.Num
			backtrack(i + 1)
			return
		}
		for n := domain.NumMinus1; n <= domain.Num11; n++ {
			if c.Color == domain.ColorWhite {
				if usedWhite[n] {
					continue
				}
			} else {
				if usedBlack[n] {
					continue
				}
			}
			if i > 0 {
				prev := cards[i-1]
				if !cardOrderLess(chosen[i-1], prev.Color, n, c.Color) {
					continue
				}
			}
			chosen[i] = n
			if c.Color == domain.ColorWhite {
				usedWhite[n] = true
			} else {
				usedBlack[n] = true
			}
			backtrack(i + 1)
			if c.Color == domain.ColorWhite {
				delete(usedWhite, n)
			} else {
				delete(usedBlack, n)
			}
			if stopped {
				return
			}
		}
	}
	backtrack(0)
	return counts, total
}

// guessOption 表示一次"猜某张牌为某个数字"的候选动作及其命中概率。
type guessOption struct {
	cardID   string
	ownerID  string
	num      domain.CardNumber
	hitProb  float64
	hitCount int
	total    int
}

// bestGuessOption 基于当前可见信息计算最佳猜牌动作。
// 平局规则：概率相同时优先目标玩家剩余暗牌更少（更接近 end）；其次按 cardID 字典序稳定排序。
func bestGuessOption(r *domain.Room, playerID string) (guessOption, bool) {
	if r == nil || r.State == nil || r.State.Players == nil {
		return guessOption{}, false
	}
	known := buildKnownNumbers(r, playerID)
	var best guessOption
	found := false
	bestHiddenLeft := 0

	pids := make([]string, 0, len(r.State.Players))
	for pid := range r.State.Players {
		pids = append(pids, pid)
	}
	sort.Strings(pids)

	for _, pid := range pids {
		if pid == playerID {
			continue
		}
		ps := r.State.Players[pid]
		if ps == nil {
			continue
		}
		hiddenLeft := 0
		for _, c := range ps.Cards {
			if c != nil && !c.IsRevealed {
				hiddenLeft++
			}
		}
		if hiddenLeft == 0 {
			continue
		}
		counts, total := enumerateOwnerDistribution(ps, known)
		if total == 0 {
			continue
		}
		ids := make([]string, 0, len(counts))
		for id := range counts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, cardID := range ids {
			dist := counts[cardID]
			var maxNum domain.CardNumber
			maxCount := -1
			for n := domain.NumMinus1; n <= domain.Num11; n++ {
				cnt, ok := dist[n]
				if !ok {
					continue
				}
				if cnt > maxCount {
					maxCount = cnt
					maxNum = n
				}
			}
			if maxCount <= 0 {
				continue
			}
			prob := float64(maxCount) / float64(total)

			replace := false
			switch {
			case !found:
				replace = true
			case prob > best.hitProb+1e-9:
				replace = true
			case math.Abs(prob-best.hitProb) <= 1e-9:
				if hiddenLeft < bestHiddenLeft {
					replace = true
				} else if hiddenLeft == bestHiddenLeft && cardID < best.cardID {
					replace = true
				}
			}
			if replace {
				best = guessOption{
					cardID:   cardID,
					ownerID:  pid,
					num:      maxNum,
					hitProb:  prob,
					hitCount: maxCount,
					total:    total,
				}
				bestHiddenLeft = hiddenLeft
				found = true
			}
		}
	}
	return best, found
}

// chooseCardToGet AI 的拿牌策略：在白/黑两色公共牌之间选择评分更高的颜色。
// 评分仅依赖于 AI 可见信息：自己手牌、对手暗牌的颜色、对手已翻牌。
func chooseCardToGet(r *domain.Room, playerID string) string {
	if r == nil || r.State == nil || r.State.BoardCards == nil || len(r.State.BoardCards) == 0 {
		return ""
	}
	whiteIDs := make([]string, 0)
	blackIDs := make([]string, 0)
	for id, c := range r.State.BoardCards {
		if c == nil {
			continue
		}
		if c.Color == domain.ColorWhite {
			whiteIDs = append(whiteIDs, id)
		} else {
			blackIDs = append(blackIDs, id)
		}
	}
	sort.Strings(whiteIDs)
	sort.Strings(blackIDs)
	if len(whiteIDs) == 0 && len(blackIDs) == 0 {
		return ""
	}
	if len(whiteIDs) == 0 {
		return blackIDs[0]
	}
	if len(blackIDs) == 0 {
		return whiteIDs[0]
	}

	oppHiddenWhite := 0
	oppHiddenBlack := 0
	myWhite := 0
	myBlack := 0
	if r.State.Players != nil {
		for pid, ps := range r.State.Players {
			if ps == nil {
				continue
			}
			for _, c := range ps.Cards {
				if c == nil {
					continue
				}
				if pid == playerID {
					if c.Color == domain.ColorWhite {
						myWhite++
					} else {
						myBlack++
					}
					continue
				}
				if c.IsRevealed {
					continue
				}
				if c.Color == domain.ColorWhite {
					oppHiddenWhite++
				} else {
					oppHiddenBlack++
				}
			}
		}
	}

	whiteScore := oppHiddenWhite * 2
	if myWhite < myBlack {
		whiteScore++
	}
	blackScore := oppHiddenBlack * 2
	if myBlack < myWhite {
		blackScore++
	}
	if whiteScore > blackScore {
		return whiteIDs[0]
	}
	if blackScore > whiteScore {
		return blackIDs[0]
	}
	if len(whiteIDs) >= len(blackIDs) {
		return whiteIDs[0]
	}
	return blackIDs[0]
}

// shouldGuessAgainInSetCard 在 setCard 状态下决定 AI 是改用 game_guess_card 继续猜，
// 还是老老实实放置新牌。
// 进入 setCard 意味着 AI 之前刚猜中且对手仍有暗牌；此时 AI 没有"待放置新牌"
// （新牌只有猜错时才会被强制翻开），所以阈值采用 0.35；下一手即可终结对手时放宽到 0.40。
func shouldGuessAgainInSetCard(r *domain.Room, playerID string) (guessOption, bool) {
	opt, ok := bestGuessOption(r, playerID)
	if !ok {
		return guessOption{}, false
	}
	threshold := 0.35
	if nextWillEndOpponent(r, opt) && threshold > 0.40 {
		threshold = 0.40
	}
	if opt.hitProb >= threshold-1e-9 {
		return opt, true
	}
	return guessOption{}, false
}

// nextWillEndOpponent 判断下一次猜中能否直接让对手全部翻开。
func nextWillEndOpponent(r *domain.Room, opt guessOption) bool {
	if r == nil || r.State == nil || r.State.Players == nil {
		return false
	}
	ps := r.State.Players[opt.ownerID]
	if ps == nil {
		return false
	}
	hidden := 0
	for _, c := range ps.Cards {
		if c != nil && !c.IsRevealed {
			hidden++
		}
	}
	return hidden == 1
}

func chooseSetForAI(r *domain.Room, playerID string) (string, int, bool) {
	ps, ok := r.State.Players[playerID]
	if !ok || ps == nil || len(ps.Cards) == 0 {
		return "", 0, false
	}
	target := ps.Cards[len(ps.Cards)-1]
	if target == nil {
		return "", 0, false
	}
	type item struct {
		id    string
		color domain.Color
		num   domain.CardNumber
		index int
	}
	var others []item
	for _, c := range ps.Cards {
		if c == nil || c.ID == target.ID {
			continue
		}
		others = append(others, item{
			id:    c.ID,
			color: c.Color,
			num:   c.Num,
			index: c.Index,
		})
	}
	sort.Slice(others, func(i, j int) bool {
		if others[i].num != others[j].num {
			return others[i].num < others[j].num
		}
		ri := 0
		if others[i].color == domain.ColorWhite {
			ri = 1
		}
		rj := 0
		if others[j].color == domain.ColorWhite {
			rj = 1
		}
		return ri < rj
	})
	pos := 0
	for i := 0; i < len(others); i++ {
		if target.Num < others[i].num {
			break
		}
		if target.Num == others[i].num {
			ti := 0
			if target.Color == domain.ColorWhite {
				ti = 1
			}
			oi := 0
			if others[i].color == domain.ColorWhite {
				oi = 1
			}
			if ti < oi {
				break
			}
		}
		pos++
	}
	return target.ID, pos, true
}

// buildAIActionMsg 根据当前 RoomStatus 选出"AI/超时"应当投递的命令 type+payload。
// 返回 ok=false 表示该状态下没有可投递的动作；调用方应在收到 ok=false 时
// 走 turn_timeout 兜底路径，避免房间死锁。
func buildAIActionMsg(r *domain.Room, playerID string, status domain.RoomStatus) (cmdType string, payload []byte, ok bool) {
	switch status {
	case domain.RoomStatusGetCard:
		cardID := chooseCardToGet(r, playerID)
		if cardID == "" {
			cardID = anyBoardCardID(r)
		}
		if cardID == "" {
			return "", nil, false
		}
		data, err := json.Marshal(map[string]interface{}{"id": cardID})
		if err != nil {
			return "", nil, false
		}
		return "game_get_card", data, true
	case domain.RoomStatusGuessCard:
		opt, ok := bestGuessOption(r, playerID)
		if !ok {
			id, num, fallback := anyOpponentHiddenCard(r, playerID)
			if !fallback {
				return "", nil, false
			}
			data, err := json.Marshal(map[string]interface{}{"id": id, "num": num})
			if err != nil {
				return "", nil, false
			}
			return "game_guess_card", data, true
		}
		data, err := json.Marshal(map[string]interface{}{
			"id":  opt.cardID,
			"num": opt.num,
		})
		if err != nil {
			return "", nil, false
		}
		return "game_guess_card", data, true
	case domain.RoomStatusSetCard:
		// AI 在 setCard 状态下也允许再猜一次：若下一手概率达阈值则发猜牌命令，
		// 否则按既有逻辑放置新牌。
		if opt, ok := shouldGuessAgainInSetCard(r, playerID); ok {
			data, err := json.Marshal(map[string]interface{}{
				"id":  opt.cardID,
				"num": opt.num,
			})
			if err == nil {
				return "game_guess_card", data, true
			}
		}
		id, index, ok := chooseSetForAI(r, playerID)
		if !ok {
			return "", nil, false
		}
		data, err := json.Marshal(map[string]interface{}{"id": id, "index": index})
		if err != nil {
			return "", nil, false
		}
		return "game_set_card", data, true
	default:
		return "", nil, false
	}
}

// anyBoardCardID 兜底：从 BoardCards 中取任一非空 ID（按字典序保证确定性）。
func anyBoardCardID(r *domain.Room) string {
	if r == nil || r.State == nil || len(r.State.BoardCards) == 0 {
		return ""
	}
	ids := make([]string, 0, len(r.State.BoardCards))
	for id, c := range r.State.BoardCards {
		if c == nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return ids[0]
}

// anyOpponentHiddenCard 兜底：从对手未翻开的牌中任取一张，num 取 Num1。
// 错猜也是一个合法动作，能继续推进游戏（自身翻牌 + 切人）。
func anyOpponentHiddenCard(r *domain.Room, playerID string) (string, domain.CardNumber, bool) {
	if r == nil || r.State == nil || r.State.Players == nil {
		return "", 0, false
	}
	pids := make([]string, 0, len(r.State.Players))
	for pid := range r.State.Players {
		pids = append(pids, pid)
	}
	sort.Strings(pids)
	for _, pid := range pids {
		if pid == playerID {
			continue
		}
		ps := r.State.Players[pid]
		if ps == nil {
			continue
		}
		for _, c := range ps.Cards {
			if c != nil && !c.IsRevealed {
				return c.ID, domain.Num1, true
			}
		}
	}
	return "", 0, false
}

// BuildTurnTimeoutCommand 思考超时时由 roomcore 调用。
// 与 MaybeRunAIIfNeeded 共用 buildAIActionMsg，但 PlayerID 用真实玩家 ID，且不修改身份。
func BuildTurnTimeoutCommand(r *domain.Room, playerID string) (roomcore.Command, bool) {
	cmdType, payload, ok := buildAIActionMsg(r, playerID, r.State.RoomStatus)
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

	roomData, ok := msg["roomData"].(map[string]interface{})
	if !ok {
		return false
	}
	currentPlayerID, ok := roomData["currentPlayer"].(string)
	if !ok || currentPlayerID == "" {
		return false
	}

	gameStatusStr, ok := roomData["gameStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := domain.RoomStatus(gameStatusStr)

	playerId, ok := msg["playerId"].(string)
	if !ok || playerId == "" || playerId != currentPlayerID {
		return false
	}
	isAI := false
	if p, ok := r.Connections[currentPlayerID]; ok && p.AI {
		isAI = true
	}
	if !isAI {
		return false
	}

	if r.AIRunning {
		return false
	}

	r.AIRunning = true

	go func() {
		defer func() {
			r.AIRunning = false
		}()
		time.Sleep(1200 * time.Millisecond)

		cmdType, payload, ok := buildAIActionMsg(r, currentPlayerID, gameStatus)
		if !ok {
			logger.Warn("AI 未生成有效动作，发送 turn_timeout 兜底",
				logger.F("room_id", r.ID),
				logger.F("player_id", currentPlayerID),
				logger.F("game_status", gameStatus))
			r.CmdCh <- domain.Command{
				Type:     "turn_timeout",
				PlayerID: playerId,
				Payload:  []byte("{}"),
				Conn:     &VirtualConn{Room: r},
			}
			return
		}

		r.CmdCh <- domain.Command{
			Type:     cmdType,
			PlayerID: playerId,
			Payload:  payload,
			Conn:     &VirtualConn{Room: r},
		}
	}()

	return true
}
