package loadtest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
)

func TestCreateRoomsRequiresToken(t *testing.T) {
	t.Setenv("ACQUIRE_LOADTEST_ENABLED", "true")
	t.Setenv("ACQUIRE_LOADTEST_TOKEN", "secret")
	gin.SetMode(gin.TestMode)

	r := gin.New()
	RegisterRoutes(r)

	reqBody := []byte(`{"prefix":"lt-auth","count":1}`)
	req := httptest.NewRequest(http.MethodPost, "/__loadtest/acquire/rooms", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if roompkg.Rooms.Len() != 0 {
		t.Fatalf("rooms len = %d, want 0", roompkg.Rooms.Len())
	}
}

func TestCreateRoomsCreatesPrefixedRooms(t *testing.T) {
	t.Setenv("ACQUIRE_LOADTEST_ENABLED", "true")
	t.Setenv("ACQUIRE_LOADTEST_TOKEN", "secret")
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		cleanupRoomsWithPrefix("lt-create")
	})

	r := gin.New()
	RegisterRoutes(r)

	reqBody := []byte(`{"prefix":"lt-create","count":2,"ownerPrefix":"lt-owner"}`)
	req := httptest.NewRequest(http.MethodPost, "/__loadtest/acquire/rooms", bytes.NewReader(reqBody))
	req.Header.Set("X-Loadtest-Token", "secret")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}

	var body struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Rooms []string `json:"rooms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Rooms) != 2 {
		t.Fatalf("rooms = %v, want 2 rooms", body.Data.Rooms)
	}
	for _, roomID := range body.Data.Rooms {
		if _, ok := roompkg.Rooms.Get(roomID); !ok {
			t.Fatalf("room %s was not created", roomID)
		}
	}
}

func TestDeleteRoomsDeletesOnlyMatchingPrefix(t *testing.T) {
	t.Setenv("ACQUIRE_LOADTEST_ENABLED", "true")
	t.Setenv("ACQUIRE_LOADTEST_TOKEN", "secret")
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		cleanupRoomsWithPrefix("lt-del")
		cleanupRoomsWithPrefix("lt-keep")
	})

	roompkg.Rooms.Set("lt-del-000001", roompkg.NewRoomService("lt-del-000001", "owner"))
	roompkg.Rooms.Set("lt-keep-000001", roompkg.NewRoomService("lt-keep-000001", "owner"))

	r := gin.New()
	RegisterRoutes(r)

	reqBody := []byte(`{"prefix":"lt-del"}`)
	req := httptest.NewRequest(http.MethodDelete, "/__loadtest/acquire/rooms", bytes.NewReader(reqBody))
	req.Header.Set("X-Loadtest-Token", "secret")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}
	if _, ok := roompkg.Rooms.Get("lt-del-000001"); ok {
		t.Fatalf("lt-del room still exists")
	}
	if _, ok := roompkg.Rooms.Get("lt-keep-000001"); !ok {
		t.Fatalf("lt-keep room was deleted")
	}
}
