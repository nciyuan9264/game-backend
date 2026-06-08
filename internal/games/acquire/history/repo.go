package history

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repo 封装 gorm 的 acquire history 仓库操作。
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// AutoMigrate 自动建表（开发期）。
func (r *Repo) AutoMigrate() error {
	return r.db.AutoMigrate(&Game{}, &GamePlayer{}, &GameEvent{})
}

// SaveCompletedGame 在一个事务里写入一整局：games + game_players + game_events。
// 仅在终局调用；中途未结束的对局不会到达此处。
func (r *Repo) SaveCompletedGame(
	ctx context.Context,
	g *Game,
	players []GamePlayer,
	events []*GameEvent,
) (int64, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(g).Error; err != nil {
			return err
		}
		for i := range players {
			players[i].GameID = g.ID
		}
		if len(players) > 0 {
			if err := tx.Create(&players).Error; err != nil {
				return err
			}
		}
		if len(events) > 0 {
			for i := range events {
				events[i].GameID = g.ID
			}
			if err := tx.CreateInBatches(events, 64).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return g.ID, nil
}

// ListByUser 列出某用户参与的对局，按 started_at desc。
func (r *Repo) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]Game, error) {
	var games []Game
	q := r.db.WithContext(ctx).
		Distinct("games.*").
		Joins("JOIN game_players ON game_players.game_id = games.id").
		Where("game_players.user_id = ?", userID).
		Order("games.started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&games).Error; err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return games, nil
	}
	ids := make([]int64, 0, len(games))
	for i := range games {
		ids = append(ids, games[i].ID)
	}
	var allPlayers []GamePlayer
	if err := r.db.WithContext(ctx).Where("game_id IN ?", ids).Find(&allPlayers).Error; err != nil {
		return nil, err
	}
	playersByGame := make(map[int64][]GamePlayer)
	for _, p := range allPlayers {
		playersByGame[p.GameID] = append(playersByGame[p.GameID], p)
	}
	for i := range games {
		games[i].Players = playersByGame[games[i].ID]
	}
	return games, nil
}

// Detail 返回一局的元信息（不含 events 详细 payload，用 EventsMeta 单独取）。
func (r *Repo) Detail(ctx context.Context, gameID int64) (*Game, []GamePlayer, []GameEvent, error) {
	var g Game
	if err := r.db.WithContext(ctx).First(&g, gameID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}
	var players []GamePlayer
	if err := r.db.WithContext(ctx).Where("game_id = ?", gameID).Order("seat_index ASC").Find(&players).Error; err != nil {
		return nil, nil, nil, err
	}
	var events []GameEvent
	if err := r.db.WithContext(ctx).Where("game_id = ?", gameID).Order("seq ASC").Find(&events).Error; err != nil {
		return nil, nil, nil, err
	}
	return &g, players, events, nil
}

// EventsUpTo 仅取该 game 的 [0, targetSeq] 事件，用于回放。
func (r *Repo) EventsUpTo(ctx context.Context, gameID int64, targetSeq int) ([]GameEvent, error) {
	var events []GameEvent
	if err := r.db.WithContext(ctx).
		Where("game_id = ? AND seq <= ?", gameID, targetSeq).
		Order("seq ASC").
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// StatsByUser 胜率聚合。
type Stats struct {
	TotalGames int     `json:"totalGames"`
	Wins       int     `json:"wins"`
	WinRate    float64 `json:"winRate"`
	AvgScore   float64 `json:"avgScore"`
}

// StatsByUser 计算某用户的胜率统计；只统计有终局（is_winner / final_score 已写入）的局。
func (r *Repo) StatsByUser(ctx context.Context, userID int64) (Stats, error) {
	var s Stats
	type row struct {
		Total int
		Wins  int
		Avg   float64
	}
	var rr row
	err := r.db.WithContext(ctx).
		Table("game_players").
		Select(`COUNT(*) AS total,
		        COUNT(*) FILTER (WHERE is_winner) AS wins,
		        COALESCE(AVG(final_score), 0) AS avg`).
		Where("user_id = ? AND final_score IS NOT NULL", userID).
		Scan(&rr).Error
	if err != nil {
		return s, err
	}
	s.TotalGames = rr.Total
	s.Wins = rr.Wins
	s.AvgScore = rr.Avg
	if s.TotalGames > 0 {
		s.WinRate = float64(s.Wins) / float64(s.TotalGames)
	}
	return s, nil
}
