package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
)

func roomTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	roompkg.Rooms.Clear()
	t.Cleanup(func() {
		for _, rs := range roompkg.Rooms.Snapshot() {
			close(rs.Room.QuitCh)
		}
		roompkg.Rooms.Clear()
	})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Set("name", "Alice")
		c.Set("avatar", "/alice.png")
		c.Set("email", "private@example.com")
	})
	r.POST("/room/create", createRoom)
	r.GET("/room/list", func(c *gin.Context) {
		c.Set("name", "")
		c.Set("avatar", "")
		listRooms(c)
	})
	r.GET("/room/lookup", func(c *gin.Context) {
		c.Set("name", "")
		c.Set("avatar", "")
		lookupRoom(c)
	})
	return r
}

func createTestRoom(t *testing.T, r *gin.Engine, body string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/room/create", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Data struct {
			RoomID string `json:"roomID"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.RoomID == "" {
		t.Fatal("missing ID")
	}
	return result.Data.RoomID
}

func TestPrivateRoomVisibleToOwnerButHiddenFromOthers(t *testing.T) {
	r := roomTestRouter(t)
	publicID := createTestRoom(t, r, "")
	privateID := createTestRoom(t, r, `{"visibility":"private"}`)
	if publicID == privateID {
		t.Fatal("simultaneous creates reused room ID")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/room/list", nil))
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(privateID)) || !bytes.Contains(rec.Body.Bytes(), []byte(publicID)) {
		t.Fatalf("owner list: %s", rec.Body.String())
	}

	other := gin.New()
	other.Use(func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("name", "Bob")
	})
	other.GET("/room/list", listRooms)
	rec = httptest.NewRecorder()
	other.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/room/list", nil))
	if rec.Code != 200 || bytes.Contains(rec.Body.Bytes(), []byte(privateID)) || !bytes.Contains(rec.Body.Bytes(), []byte(publicID)) {
		t.Fatalf("other player list: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/room/lookup?roomID="+privateID, nil))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var result struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if string(result.Data["visibility"]) != `"private"` || string(result.Data["ownerName"]) != `"Alice"` {
		t.Fatal(rec.Body.String())
	}
	for _, key := range []string{"players", "playerData", "email", "state", "boardTiles", "stocks", "money"} {
		if _, ok := result.Data[key]; ok {
			t.Fatalf("private data returned: %s", key)
		}
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/room/lookup?roomID="+privateID[:4], nil))
	if rec.Code != 404 {
		t.Fatalf("partial ID lookup status %d", rec.Code)
	}
}

func TestCreateRejectsInvalidSettingsAndEnforcesLimit(t *testing.T) {
	r := roomTestRouter(t)
	for _, body := range []string{`{"visibility":"friends"}`, `{"visibility":""}`, `{`, `{} {}`} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/room/create", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Fatalf("body %q: %d", body, rec.Code)
		}
	}
	for i := 0; i < 5; i++ {
		createTestRoom(t, r, `{"visibility":"private"}`)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/room/create", strings.NewReader(`{}`)))
	if rec.Code != 409 {
		t.Fatalf("ownership limit: %d", rec.Code)
	}
}

func TestAcquireWebSocketRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	InitRouter(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws?roomID=test&userID=forged", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous WS status: %d", rec.Code)
	}
}
