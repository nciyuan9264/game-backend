package game

import (
	"encoding/json"
	"sort"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
)

const (
	EndConditionCompanySize      = "company_size"
	EndConditionAllCompaniesSafe = "all_companies_safe"

	acquireBoardTileCount = 108
)

type EndVotePayload struct {
	Allow *bool `json:"allow"`
}

type endCompanyCandidate struct {
	Name  string
	Tiles int
}

func ParseEndVotePayload(payload []byte) (bool, bool) {
	var p EndVotePayload
	if json.Unmarshal(payload, &p) != nil || p.Allow == nil {
		return false, false
	}
	return *p.Allow, true
}

func HandleEndVoteMessage(r *domain.Room, cmd domain.Command) {
	if r == nil || r.State == nil || !r.State.EndGame.Eligible {
		return
	}
	allow, ok := ParseEndVotePayload(cmd.Payload)
	if !ok {
		return
	}
	ensureEndVotes(r)
	r.State.EndGame.Votes[cmd.PlayerID] = allow
}

// RefreshEndGameEligibility latches the first attained end condition.
func RefreshEndGameEligibility(r *domain.Room, companyTiles map[string]int) {
	if r == nil || r.State == nil {
		return
	}
	if r.State.EndGame.Eligible {
		ensureEndVotes(r)
		return
	}

	candidates := make([]endCompanyCandidate, 0, len(companyTiles))
	allEstablishedSafe := len(companyTiles) > 0
	for name, tiles := range companyTiles {
		if tiles >= 41 {
			candidates = append(candidates, endCompanyCandidate{Name: name, Tiles: tiles})
		}
		if tiles < 11 {
			allEstablishedSafe = false
		}
	}

	end := domain.EndGameState{}
	if len(candidates) > 0 {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Tiles != candidates[j].Tiles {
				return candidates[i].Tiles > candidates[j].Tiles
			}
			return candidates[i].Name < candidates[j].Name
		})
		end.Eligible = true
		end.Condition = EndConditionCompanySize
		end.Company = candidates[0].Name
		end.TileCount = candidates[0].Tiles
	} else if allEstablishedSafe {
		end.Eligible = true
		end.Condition = EndConditionAllCompaniesSafe
	}
	if !end.Eligible {
		return
	}

	end.Votes = make(map[string]bool, len(r.Connections))
	for playerID, pc := range r.Connections {
		end.Votes[playerID] = pc != nil && pc.AI
	}
	r.State.EndGame = end
}

func AllEndVotesApproved(r *domain.Room) bool {
	if r == nil || r.State == nil || !r.State.EndGame.Eligible {
		return false
	}
	for playerID, pc := range r.Connections {
		if pc != nil && !pc.AI && !r.State.EndGame.Votes[playerID] {
			return false
		}
	}
	return true
}

func IsBoardFull(r *domain.Room) bool {
	if r == nil || r.State == nil || len(r.State.BoardTiles) < acquireBoardTileCount {
		return false
	}
	placed := 0
	for _, tile := range r.State.BoardTiles {
		if tile != nil && tile.Belong != "" {
			placed++
		}
	}
	return placed >= acquireBoardTileCount
}

func ShouldFinalizeEnd(r *domain.Room, turnCompleted bool) bool {
	if !turnCompleted || r == nil || r.State == nil || r.State.RoomStatus == domain.RoomStatusEnd {
		return false
	}
	return IsBoardFull(r) || AllEndVotesApproved(r)
}

func ensureEndVotes(r *domain.Room) {
	if r.State.EndGame.Votes == nil {
		r.State.EndGame.Votes = make(map[string]bool, len(r.Connections))
	}
	for playerID, pc := range r.Connections {
		if _, exists := r.State.EndGame.Votes[playerID]; !exists {
			r.State.EndGame.Votes[playerID] = pc != nil && pc.AI
		} else if pc != nil && pc.AI {
			r.State.EndGame.Votes[playerID] = true
		}
	}
}
