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
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
)

func TestWebSocketUsesAuthenticatedIdentityAndAcknowledgesCommands(t *testing.T) {
	gin.SetMode(gin.TestMode)
	room := roompkg.NewRoomService("identity-test", "Alice-42")
	roompkg.Rooms.Set(room.Room.ID, room)
	go room.Run()
	defer func() {
		close(room.Room.QuitCh)
		roompkg.Rooms.Delete(room.Room.ID)
	}()

	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Set("name", "Alice")
		c.Set("avatar", "/alice.png")
		HandleWebSocket(c)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?roomID=identity-test&userID=forged-owner"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["playerID"] != "Alice-42" {
		t.Fatalf("query identity was trusted: %+v", response)
	}
	players := response["players"].(map[string]any)
	if players["Alice-42"].(map[string]any)["name"] != "Alice" {
		t.Fatal("authenticated profile was not synchronized")
	}

	if err := conn.WriteJSON(map[string]any{"type": "disconnect", "requestId": "invalid-1"}); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["code"] != "invalid_command" || response["requestId"] != "invalid-1" {
		t.Fatalf("internal command accepted: %+v", response)
	}

	if err := conn.WriteJSON(map[string]any{
		"type": "match_add_ai", "requestId": "add-ai-1", "phaseVersion": response["phaseVersion"],
		"payload": map[string]any{"seatIndex": 1},
	}); err != nil {
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
	if response["requestId"] != "add-ai-1" {
		t.Fatalf("missing request acknowledgement: %+v", response)
	}

	duplicate, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := duplicate.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	_ = duplicate.Close()
	if response["code"] != "identity_conflict" {
		t.Fatalf("duplicate response: %+v", response)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := room.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 2 || !snapshot.Players[0].Online {
		t.Fatalf("duplicate changed active room members: %+v", snapshot.Players)
	}
}

func TestCheckOriginUsesExplicitAllowlist(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGINS", "https://preview.example.com")
	for origin, allowed := range map[string]bool{
		"":                             true,
		"https://board.gamebus.online": true,
		"http://localhost:5173":        true,
		"https://preview.example.com":  true,
		"https://attacker.example.com": false,
	} {
		request := httptest.NewRequest(http.MethodGet, "/ws", nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if got := checkOrigin(request); got != allowed {
			t.Fatalf("origin %q allowed = %v, want %v", origin, got, allowed)
		}
	}
}
