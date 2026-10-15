package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func mergeTestRoom() *domain.Room {
	base := roomcore.NewBase("merge-test", 8)
	board := make(map[string]*domain.Tile, 108)
	for col := 1; col <= 12; col++ {
		for row := 'A'; row <= 'I'; row++ {
			id := fmt.Sprintf("%d%c", col, row)
			board[id] = &domain.Tile{ID: id}
		}
	}
	return &domain.Room{
		Base: base,
		State: &domain.GameState{
			LastTileKey: "2A",
			RoomStatus:  domain.RoomStatusSetTile,
			BoardTiles:  board,
			Players:     map[string]*domain.PlayerState{},
			Companies: map[string]*domain.CompanyState{
				"American": {Name: "American", Tiles: 2, StockTotal: 25, StockPrice: 300},
				"Tower":    {Name: "Tower", Tiles: 3, StockTotal: 25, StockPrice: 300},
			},
		},
	}
}

func TestHandleMergeProcessBuildsHoldersFromPlayerState(t *testing.T) {
	r := mergeTestRoom()
	r.State.Players["alice"] = &domain.PlayerState{Stocks: map[string]int{"American": 3}}
	r.State.Players["bob"] = &domain.PlayerState{Stocks: map[string]int{"American": 1}}

	if err := handleMergeProcess(r, "Tower", []string{"American"}, map[string]int{"American": 2}); err != nil {
		t.Fatal(err)
	}

	settlement := r.State.MergeSettleData["American"]
	if r.State.RoomStatus != domain.RoomStatusMergingSettle {
		t.Fatalf("room status = %q, want mergingSettle", r.State.RoomStatus)
	}
	if len(settlement.Hoders) != 2 || settlement.Hoders[0] != "alice" || settlement.Hoders[1] != "bob" {
		t.Fatalf("holders = %v, want [alice bob]", settlement.Hoders)
	}
	if settlement.Dividends["alice"] == 0 || settlement.Dividends["bob"] == 0 {
		t.Fatalf("dividends = %v, want both shareholders rewarded", settlement.Dividends)
	}
}

func TestHandleMergeProcessCompletesImmediatelyWithoutHolders(t *testing.T) {
	r := mergeTestRoom()
	r.State.Players["alice"] = &domain.PlayerState{Stocks: map[string]int{}}
	r.State.BoardTiles["1A"].Belong = "American"
	r.State.BoardTiles["2A"].Belong = "Blank"
	r.State.BoardTiles["3A"].Belong = "Tower"

	if err := handleMergeProcess(r, "Tower", []string{"American"}, map[string]int{"American": 2}); err != nil {
		t.Fatal(err)
	}

	if r.State.RoomStatus != domain.RoomStatusBuyStock {
		t.Fatalf("room status = %q, want buyStock", r.State.RoomStatus)
	}
	if len(r.State.MergeSettleData) != 0 {
		t.Fatalf("merge settlement was not cleared: %+v", r.State.MergeSettleData)
	}
	for _, tileID := range []string{"1A", "2A", "3A"} {
		if got := r.State.BoardTiles[tileID].Belong; got != "Tower" {
			t.Fatalf("tile %s belongs to %q, want Tower", tileID, got)
		}
	}
}

func TestHandleMergingSettleRecordsMultiCompanyReceipt(t *testing.T) {
	r := mergeTestRoom()
	r.State.RoomStatus = domain.RoomStatusMergingSettle
	r.State.MergeMainCompany = "Tower"
	r.State.Companies["Festival"] = &domain.CompanyState{Name: "Festival", Tiles: 3, StockTotal: 25, StockPrice: 400}
	r.State.Players["alice"] = &domain.PlayerState{Money: 6000, Stocks: map[string]int{"American": 4, "Festival": 3, "Tower": 0}}
	r.State.Players["bob"] = &domain.PlayerState{Money: 6000, Stocks: map[string]int{"American": 1}}
	r.State.MergeSettleData = map[string]domain.SettleData{
		"American": {
			Hoders: []string{"alice", "bob"}, Dividends: map[string]int{"alice": 3000},
			Settled: map[string]domain.MergingSettleResult{},
		},
		"Festival": {
			Hoders: []string{"alice"}, Dividends: map[string]int{"alice": 4000},
			Settled: map[string]domain.MergingSettleResult{},
		},
	}
	payload, err := json.Marshal(MergingSettlePayload{Actions: []domain.MergingSettleItem{
		{Company: "American", SellAmount: 1, ExchangeAmount: 2},
		{Company: "Festival", SellAmount: 0, ExchangeAmount: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}

	HandleMergingSettleMessage(r, domain.Command{PlayerID: "alice", Payload: payload})

	if r.State.RoomStatus != domain.RoomStatusMergingSettle {
		t.Fatalf("room status = %q, want mergingSettle while bob is pending", r.State.RoomStatus)
	}
	american := r.State.MergeSettleData["American"]
	festival := r.State.MergeSettleData["Festival"]
	if len(american.Hoders) != 1 || american.Hoders[0] != "bob" || len(festival.Hoders) != 0 {
		t.Fatalf("pending holders not updated: American=%v Festival=%v", american.Hoders, festival.Hoders)
	}
	if got := american.Settled["alice"]; got.HoldingBefore != 4 || got.SellAmount != 1 || got.ExchangeAmount != 2 {
		t.Fatalf("American receipt = %+v", got)
	}
	if got := festival.Settled["alice"]; got.HoldingBefore != 3 || got.SellAmount != 0 || got.ExchangeAmount != 2 {
		t.Fatalf("Festival receipt = %+v", got)
	}
}
