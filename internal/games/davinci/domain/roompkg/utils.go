package roompkg

import (
	"fmt"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
)

func genAIPlayerID(r *domain.Room) string {
	for i := 1; ; i++ {
		id := fmt.Sprintf("ai_%03d", i)
		if _, ok := r.Connections[id]; !ok {
			return id
		}
	}
}
