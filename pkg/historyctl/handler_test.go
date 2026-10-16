package historyctl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
)

type leaderboardStoreStub struct {
	called   bool
	gameType string
	sort     gamehistory.LeaderboardSort
	limit    int
	offset   int
}

func (s *leaderboardStoreStub) ListByUser(context.Context, int64, string, int, int) ([]gamehistory.Game, error) {
	return nil, nil
}

func (s *leaderboardStoreStub) Detail(context.Context, int64, string) (*gamehistory.Game, []gamehistory.GamePlayer, []gamehistory.GameEvent, error) {
	return nil, nil, nil, nil
}

func (s *leaderboardStoreStub) StatsByUser(context.Context, int64, string) (gamehistory.Stats, error) {
	return gamehistory.Stats{}, nil
}

func (s *leaderboardStoreStub) Leaderboard(_ context.Context, gameType string, sort gamehistory.LeaderboardSort, limit, offset int) ([]gamehistory.LeaderboardEntry, error) {
	s.called = true
	s.gameType = gameType
	s.sort = sort
	s.limit = limit
	s.offset = offset
	return []gamehistory.LeaderboardEntry{{UserID: 1, PlayerID: "alice", TotalGames: 3}}, nil
}

func TestLeaderboardHandlerPassesSortLimitAndOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &leaderboardStoreStub{}
	router := gin.New()
	router.GET("/ranking/leaderboard", MakeLeaderboardHandler(Options{
		Store:           store,
		DefaultGameType: "acquire",
	}))

	req := httptest.NewRequest(http.MethodGet, "/ranking/leaderboard?sort=totalGames&limit=8&offset=16", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !store.called {
		t.Fatal("Leaderboard was not called")
	}
	if store.gameType != "acquire" || store.sort != gamehistory.LeaderboardSortTotalGames || store.limit != 8 || store.offset != 16 {
		t.Fatalf("args = gameType:%q sort:%q limit:%d offset:%d", store.gameType, store.sort, store.limit, store.offset)
	}
}

func TestLeaderboardHandlerRejectsInvalidSort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &leaderboardStoreStub{}
	router := gin.New()
	router.GET("/ranking/leaderboard", MakeLeaderboardHandler(Options{
		Store:           store,
		DefaultGameType: "acquire",
	}))

	req := httptest.NewRequest(http.MethodGet, "/ranking/leaderboard?sort=score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if store.called {
		t.Fatal("Leaderboard should not be called for invalid sort")
	}
}
