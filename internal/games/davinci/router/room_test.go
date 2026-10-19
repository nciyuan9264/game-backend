package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
)

func roomTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	roompkg.Rooms.Clear()
	t.Cleanup(func() {
		for _, room := range roompkg.Rooms.Snapshot() {
			close(room.Room.QuitCh)
		}
		roompkg.Rooms.Clear()
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Set("name", "Alice")
		c.Set("avatar", "/alice.png")
	})
	router.POST("/room/create", createRoom)
	router.GET("/room/list", listRooms)
	return router
}

func TestCreateAndListUseAuthenticatedProfile(t *testing.T) {
	router := roomTestRouter(t)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/room/create", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var createResponse struct {
		Data struct {
			RoomID string `json:"roomID"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/room/list", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range [][]byte{
		[]byte(createResponse.Data.RoomID),
		[]byte(`"ownerID":"Alice-42"`),
		[]byte(`"ownerName":"Alice"`),
		[]byte(`"ownerAvatar":"/alice.png"`),
		[]byte(`"maxPlayers":2`),
		[]byte(`"isMember":true`),
	} {
		if !bytes.Contains(recorder.Body.Bytes(), expected) {
			t.Fatalf("list response missing %s: %s", expected, recorder.Body.String())
		}
	}
}

func TestDavinciWebSocketRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	InitRouter(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws?roomID=test&userID=forged", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous WS status = %d", recorder.Code)
	}
}
