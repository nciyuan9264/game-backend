package loadtest

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
)

const (
	loadtestEnabledEnv   = "ACQUIRE_LOADTEST_ENABLED"
	loadtestTokenEnv     = "ACQUIRE_LOADTEST_TOKEN"
	loadtestTokenHeader  = "X-Loadtest-Token"
	maxCreateRooms       = 500
	statsSnapshotTimeout = time.Second
)

type createRoomsRequest struct {
	Prefix      string `json:"prefix"`
	Count       int    `json:"count"`
	OwnerPrefix string `json:"ownerPrefix"`
}

type prefixRequest struct {
	Prefix string `json:"prefix"`
}

// Enabled reports whether guarded loadtest routes should be registered.
func Enabled() bool {
	return os.Getenv(loadtestEnabledEnv) == "true"
}

// RegisterRoutes registers Acquire loadtest helper routes behind a token guard.
func RegisterRoutes(r gin.IRouter) {
	group := r.Group("/__loadtest/acquire")
	group.Use(requireToken())
	group.POST("/rooms", createRooms)
	group.DELETE("/rooms", deleteRooms)
	group.GET("/stats", stats)
}

func requireToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := os.Getenv(loadtestTokenEnv)
		if token == "" || c.GetHeader(loadtestTokenHeader) != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status_code": http.StatusUnauthorized,
				"error":       "unauthorized",
			})
			return
		}
		c.Next()
	}
}

func createRooms(c *gin.Context) {
	var req createRoomsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status_code": http.StatusBadRequest, "error": "invalid_json"})
		return
	}
	if !strings.HasPrefix(req.Prefix, "lt-") {
		c.JSON(http.StatusBadRequest, gin.H{"status_code": http.StatusBadRequest, "error": "invalid_prefix"})
		return
	}
	if req.Count <= 0 || req.Count > maxCreateRooms {
		c.JSON(http.StatusBadRequest, gin.H{"status_code": http.StatusBadRequest, "error": "invalid_count"})
		return
	}
	if req.OwnerPrefix == "" {
		req.OwnerPrefix = req.Prefix + "-owner"
	}

	rooms := make([]string, 0, req.Count)
	for i := 1; i <= req.Count; i++ {
		roomID := fmt.Sprintf("%s-%06d", req.Prefix, i)
		ownerID := fmt.Sprintf("%s-%06d", req.OwnerPrefix, i)
		rs := roompkg.NewRoomService(roomID, ownerID)
		roompkg.Rooms.Set(roomID, rs)
		go rs.Run()
		rooms = append(rooms, roomID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"data":        gin.H{"rooms": rooms},
	})
}

func deleteRooms(c *gin.Context) {
	var req prefixRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status_code": http.StatusBadRequest, "error": "invalid_json"})
		return
	}
	if !strings.HasPrefix(req.Prefix, "lt-") {
		c.JSON(http.StatusBadRequest, gin.H{"status_code": http.StatusBadRequest, "error": "invalid_prefix"})
		return
	}
	deleted := cleanupRoomsWithPrefix(req.Prefix)
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"data":        gin.H{"deleted": deleted},
	})
}

func stats(c *gin.Context) {
	snapshot := roompkg.Rooms.Snapshot()
	roomStatus := map[string]int{}
	connections := 0
	onlineHumans := 0
	aiPlayers := 0
	snapshotErrors := 0
	ctx, cancel := context.WithTimeout(c.Request.Context(), statsSnapshotTimeout)
	defer cancel()

	for _, rs := range snapshot {
		if rs == nil {
			continue
		}
		roomSnapshot, err := rs.Snapshot(ctx)
		if err != nil {
			snapshotErrors++
			continue
		}
		roomStatus[roomSnapshot.Status]++
		for _, pc := range roomSnapshot.Players {
			connections++
			if pc.AI {
				aiPlayers++
			} else if pc.Online {
				onlineHumans++
			}
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"data": gin.H{
			"rooms":          len(snapshot),
			"connections":    connections,
			"onlineHumans":   onlineHumans,
			"aiPlayers":      aiPlayers,
			"snapshotErrors": snapshotErrors,
			"roomStatus":     roomStatus,
			"runtime": gin.H{
				"goroutines":     runtime.NumGoroutine(),
				"heapAllocBytes": mem.HeapAlloc,
			},
		},
	})
}

func cleanupRoomsWithPrefix(prefix string) int {
	deleted := 0
	for roomID, rs := range roompkg.Rooms.Snapshot() {
		if !strings.HasPrefix(roomID, prefix) {
			continue
		}
		if rs != nil && rs.Room != nil {
			select {
			case <-rs.Room.QuitCh:
			default:
				close(rs.Room.QuitCh)
			}
		}
		roompkg.Rooms.Delete(roomID)
		deleted++
	}
	return deleted
}
