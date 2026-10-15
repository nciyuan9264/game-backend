package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
)

func TestWebSocketUsesAuthenticatedIdentityAndRejectsInternalCommands(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rs := roompkg.NewRoomService("identity-test", "Alice-42")
	roompkg.Rooms.Set(rs.Room.ID, rs)
	go rs.Run()
	defer func() { close(rs.Room.QuitCh); roompkg.Rooms.Delete(rs.Room.ID) }()
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Set("name", "Alice")
		c.Set("avatar", "/alice.png")
		HandleWebSocket(c)
	})
	server := httptest.NewServer(r)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?roomID=identity-test&userID=forged-owner"
	conn, _, err := websocket.DefaultDialer.Dial(url, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var sync map[string]any
	if err := conn.ReadJSON(&sync); err != nil {
		t.Fatal(err)
	}
	if sync["playerID"] != "Alice-42" {
		t.Fatalf("query identity trusted: %+v", sync)
	}
	players := sync["players"].(map[string]any)
	if players["Alice-42"].(map[string]any)["name"] != "Alice" {
		t.Fatal("public profile not synced")
	}
	if err := conn.WriteJSON(map[string]any{"type": "disconnect", "requestId": "invalid-1"}); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["code"] != "invalid_command" || response["requestId"] != "invalid-1" {
		t.Fatalf("internal command accepted: %+v", response)
	}

	duplicate, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := duplicate.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	duplicate.Close()
	if response["code"] != "identity_conflict" {
		t.Fatalf("duplicate: %+v", response)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := rs.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 1 || !snapshot.Players[0].Online {
		t.Fatal("duplicate disconnected existing player")
	}
	if err := conn.WriteJSON(map[string]any{"type": "room_update_settings", "requestId": "setting-1", "payload": map[string]string{"visibility": "private"}}); err != nil {
		t.Fatal(err)
	}
	for {
		if err := conn.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response["type"] == "command_result" {
			break
		}
	}
	if response["requestId"] != "setting-1" {
		t.Fatal("request acknowledgement missing")
	}
	snapshot, err = rs.Snapshot(ctx)
	if err != nil || snapshot.Visibility != "private" {
		t.Fatalf("settings failed: %+v %v", snapshot, err)
	}
}

func TestClientAllowsEndVoteCommand(t *testing.T) {
	if !clientCommandAllowed("game_end_vote") {
		t.Fatal("game_end_vote was rejected by websocket allowlist")
	}
}
