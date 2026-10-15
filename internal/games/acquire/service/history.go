package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/replay"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

const (
	defaultSnapshotPageLimit = 20
	maxSnapshotPageLimit     = 50
)

type snapshotPageLoad struct {
	snapshots      []*replay.Snapshot
	totalEvents    int
	legacyFallback bool
	dbDuration     time.Duration
	replayDuration time.Duration
}

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
	targetSeq, err := strconv.Atoi(c.DefaultQuery("seq", "-1"))
	if err != nil || targetSeq < -1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid seq"})
		return
	}

	offset := targetSeq + 1
	startedAt := time.Now()
	page, found, err := loadSnapshotPage(c.Request.Context(), id, offset, 1)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if len(page.snapshots) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	logSnapshotPage(id, offset, 1, page, time.Since(startedAt))
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "ok",
		"data":        page.snapshots[0],
	})
}

// HistoryGameSnapshots GET /history/game/:id/snapshots?offset=N&limit=N
// 始终返回分页响应；无参数时默认只返回前 20 帧。
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
	offset, limit, err := parseSnapshotPageParams(c.Query("offset"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startedAt := time.Now()
	page, found, err := loadSnapshotPage(c.Request.Context(), id, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	totalSnapshots := page.totalEvents + 1
	nextOffset, hasMore := snapshotPageProgress(totalSnapshots, offset, len(page.snapshots))
	logSnapshotPage(id, offset, limit, page, time.Since(startedAt))
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "ok",
		"data": gin.H{
			"totalEvents":    page.totalEvents,
			"totalSnapshots": totalSnapshots,
			"offset":         offset,
			"limit":          limit,
			"nextOffset":     nextOffset,
			"hasMore":        hasMore,
			"snapshots":      page.snapshots,
		},
	})
}

func parseSnapshotPageParams(offsetValue, limitValue string) (int, int, error) {
	offset := 0
	limit := defaultSnapshotPageLimit
	var err error

	if offsetValue != "" {
		offset, err = strconv.Atoi(offsetValue)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("invalid offset")
		}
	}
	if limitValue != "" {
		limit, err = strconv.Atoi(limitValue)
		if err != nil || limit <= 0 || limit > maxSnapshotPageLimit {
			return 0, 0, fmt.Errorf("invalid limit")
		}
	}
	return offset, limit, nil
}

func snapshotEventWindow(offset, limit int) (int, int) {
	if offset == 0 {
		if limit <= 1 {
			return 0, 0
		}
		return 0, limit - 1
	}
	return offset - 1, limit
}

func snapshotPageProgress(totalSnapshots, offset, returned int) (int, bool) {
	nextOffset := offset + returned
	if nextOffset > totalSnapshots {
		nextOffset = totalSnapshots
	}
	return nextOffset, nextOffset < totalSnapshots
}

func loadSnapshotPage(ctx context.Context, gameID int64, offset, limit int) (*snapshotPageLoad, bool, error) {
	eventOffset, eventLimit := snapshotEventWindow(offset, limit)
	dbStartedAt := time.Now()
	g, players, totalEvents, pageEvents, err := roompkg.HistoryRepo.ReplayPage(
		ctx,
		gameID,
		"acquire",
		eventOffset,
		eventLimit,
	)
	dbDuration := time.Since(dbStartedAt)
	if err != nil {
		return nil, false, err
	}
	if g == nil {
		return nil, false, nil
	}

	legacyFallback := false
	var fallbackEvents []gamehistory.GameEvent
	for i := range pageEvents {
		if len(pageEvents[i].StateBlob) > 0 || len(pageEvents[i].StateSnapshot) > 0 {
			continue
		}
		legacyFallback = true
		fallbackStartedAt := time.Now()
		fallbackEvents, err = roompkg.HistoryRepo.ReplayEventsWithoutSnapshots(ctx, gameID)
		dbDuration += time.Since(fallbackStartedAt)
		if err != nil {
			return nil, false, err
		}
		break
	}

	replayStartedAt := time.Now()
	snapshots, err := replay.ReplayPage(
		g,
		players,
		totalEvents,
		offset,
		limit,
		pageEvents,
		fallbackEvents,
	)
	replayDuration := time.Since(replayStartedAt)
	if err != nil {
		return nil, false, err
	}

	return &snapshotPageLoad{
		snapshots:      snapshots,
		totalEvents:    totalEvents,
		legacyFallback: legacyFallback,
		dbDuration:     dbDuration,
		replayDuration: replayDuration,
	}, true, nil
}

func logSnapshotPage(gameID int64, offset, limit int, page *snapshotPageLoad, duration time.Duration) {
	logger.Info("acquire replay page loaded",
		logger.F("game_id", gameID),
		logger.F("offset", offset),
		logger.F("limit", limit),
		logger.F("returned", len(page.snapshots)),
		logger.F("total_events", page.totalEvents),
		logger.F("legacy_fallback", page.legacyFallback),
		logger.F("db_ms", page.dbDuration.Milliseconds()),
		logger.F("replay_ms", page.replayDuration.Milliseconds()),
		logger.F("duration_ms", duration.Milliseconds()))
}
