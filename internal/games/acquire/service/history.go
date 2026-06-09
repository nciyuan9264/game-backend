package service

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/replay"
)

// HistoryGameSnapshot GET /history/game/:id/snapshot?seq=N
// 中间件保护下未登录直接 401，无需 handler 内部处理。
func HistoryGameSnapshot(c *gin.Context) {
	if roompkg.HistoryRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history disabled"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	seqStr := c.DefaultQuery("seq", "-1")
	targetSeq, err := strconv.Atoi(seqStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid seq"})
		return
	}

	g, players, events, err := roompkg.HistoryRepo.Detail(c.Request.Context(), id, "acquire")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if g == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// 注意：必须把全量 events 传给 ReplayTo——引擎内部会按 targetSeq 切片决定回放到哪一步，
	// 同时利用未来的 placeTile 事件预测玩家"接下来 5 张牌"。
	// 不要在这里改用 EventsUpTo 把未来事件砍掉，否则 snapshot.players[*].tiles 会为空。
	snap, err := replay.ReplayTo(g, players, events, targetSeq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "ok",
		"data":        snap,
	})
}

// HistoryGameSnapshots GET /history/game/:id/snapshots
// 一次性返回该局所有回合的快照数组，前端拉取一次即可在本地平滑切换，无需逐帧请求。
func HistoryGameSnapshots(c *gin.Context) {
	if roompkg.HistoryRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history disabled"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	g, players, events, err := roompkg.HistoryRepo.Detail(c.Request.Context(), id, "acquire")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if g == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	snaps, err := replay.ReplayAll(g, players, events)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "ok",
		"data": gin.H{
			"totalEvents": len(events),
			"snapshots":   snaps,
		},
	})
}
