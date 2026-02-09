package roompkg

import (
	"fmt"
	"go-game/domain/domain"
)

func genAIPlayerID(r *domain.Room) string {
	for i := 1; ; i++ {
		id := fmt.Sprintf("ai_%03d", i)
		if _, ok := r.Players[id]; !ok {
			return id
		}
	}
}
