// Package historyctl provides shared HTTP handlers for game history.
package historyctl

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
)

type HistoryStore interface {
	ListByUser(ctx context.Context, userID int64, gameType string, limit, offset int) ([]gamehistory.Game, error)
	Detail(ctx context.Context, gameID int64, gameType string) (*gamehistory.Game, []gamehistory.GamePlayer, []gamehistory.GameEvent, error)
	StatsByUser(ctx context.Context, userID int64, gameType string) (gamehistory.Stats, error)
	Leaderboard(ctx context.Context, gameType string, limit, offset int) ([]gamehistory.LeaderboardEntry, error)
}

type Options struct {
	Store           HistoryStore
	DefaultGameType string
	AllowSnapshot   bool
	SnapshotHandler gin.HandlerFunc
}

func currentUserID(c *gin.Context) (int64, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case uint:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

func resolveGameType(c *gin.Context, defaultGameType string) (string, bool) {
	gameType := c.DefaultQuery("game_type", defaultGameType)
	switch gameType {
	case "acquire", "davinci", "splendor":
		return gameType, true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid game_type"})
		return "", false
	}
}

func MakeListGamesHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history disabled"})
			return
		}
		uid, ok := currentUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		gameType, ok := resolveGameType(c, opts.DefaultGameType)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		games, err := opts.Store.ListByUser(c.Request.Context(), uid, gameType, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "ok",
			"data":        gin.H{"games": games},
		})
	}
}

func MakeGameDetailHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history disabled"})
			return
		}
		gameType, ok := resolveGameType(c, opts.DefaultGameType)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		g, players, events, err := opts.Store.Detail(c.Request.Context(), id, gameType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if g == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		type eventMeta struct {
			Seq      int    `json:"seq"`
			PlayerID string `json:"playerID"`
			CmdType  string `json:"cmdType"`
		}
		metas := make([]eventMeta, 0, len(events))
		for _, e := range events {
			metas = append(metas, eventMeta{Seq: e.Seq, PlayerID: e.PlayerID, CmdType: e.CmdType})
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "ok",
			"data": gin.H{
				"game":    g,
				"players": players,
				"events":  metas,
			},
		})
	}
}

func MakeStatsHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history disabled"})
			return
		}
		uid, ok := currentUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		gameType, ok := resolveGameType(c, opts.DefaultGameType)
		if !ok {
			return
		}
		stats, err := opts.Store.StatsByUser(c.Request.Context(), uid, gameType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "ok",
			"data":        stats,
		})
	}
}

func MakeSnapshotHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := resolveGameType(c, opts.DefaultGameType); !ok {
			return
		}
		if !opts.AllowSnapshot || opts.SnapshotHandler == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "snapshot is not supported for this game"})
			return
		}
		opts.SnapshotHandler(c)
	}
}

// MakeLeaderboardHandler 返回全玩家排行榜（公开，无需登录）。
func MakeLeaderboardHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ranking disabled"})
			return
		}
		gameType, ok := resolveGameType(c, opts.DefaultGameType)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		entries, err := opts.Store.Leaderboard(c.Request.Context(), gameType, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "ok",
			"data": gin.H{
				"gameType": gameType,
				"entries":  entries,
			},
		})
	}
}
