package gamehistory

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repo encapsulates shared game history persistence.
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// AutoMigrate migrates the shared history tables.
func (r *Repo) AutoMigrate() error {
	return r.db.AutoMigrate(&Game{}, &GamePlayer{}, &GameEvent{})
}

// SaveCompletedGame writes one completed game. events can be empty for games
// that do not record detailed steps.
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

// ListByUser lists games by user and game type.
func (r *Repo) ListByUser(ctx context.Context, userID int64, gameType string, limit, offset int) ([]Game, error) {
	var games []Game
	q := r.db.WithContext(ctx).
		Distinct("games.*").
		Joins("JOIN game_players ON game_players.game_id = games.id").
		Where("game_players.user_id = ? AND games.game_type = ?", userID, gameType).
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
	if err := r.db.WithContext(ctx).Where("game_id IN ?", ids).Order("seat_index ASC").Find(&allPlayers).Error; err != nil {
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

// Detail returns one game with players and events. gameType scopes access.
func (r *Repo) Detail(ctx context.Context, gameID int64, gameType string) (*Game, []GamePlayer, []GameEvent, error) {
	var g Game
	if err := r.db.WithContext(ctx).Where("id = ? AND game_type = ?", gameID, gameType).First(&g).Error; err != nil {
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

// EventsUpTo returns events in [0, targetSeq].
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

type Stats struct {
	TotalGames int     `json:"totalGames"`
	Wins       int     `json:"wins"`
	WinRate    float64 `json:"winRate"`
	AvgScore   float64 `json:"avgScore"`
}

// StatsByUser computes win-rate stats scoped by game type.
func (r *Repo) StatsByUser(ctx context.Context, userID int64, gameType string) (Stats, error) {
	var s Stats
	type row struct {
		Total int
		Wins  int
		Avg   float64
	}
	var rr row
	err := r.db.WithContext(ctx).
		Table("game_players").
		Joins("JOIN games ON games.id = game_players.game_id").
		Select(`COUNT(*) AS total,
		        COUNT(*) FILTER (WHERE game_players.is_winner) AS wins,
		        COALESCE(AVG(game_players.final_score), 0) AS avg`).
		Where("game_players.user_id = ? AND games.game_type = ? AND games.ended_at IS NOT NULL", userID, gameType).
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

// LeaderboardEntry 全玩家排行榜的一行。Wins/WinRate 仅 davinci 填充；AvgRank 仅 acquire 填充。
type LeaderboardEntry struct {
	UserID     int64    `json:"userID"`
	PlayerID   string   `json:"playerID"`
	TotalGames int      `json:"totalGames"`
	Wins       *int     `json:"wins,omitempty"`
	WinRate    *float64 `json:"winRate,omitempty"`
	AvgRank    *float64 `json:"avgRank,omitempty"`
}

// Leaderboard 返回指定 game_type 的全玩家聚合排行榜。
func (r *Repo) Leaderboard(ctx context.Context, gameType string, limit, offset int) ([]LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	switch gameType {
	case "davinci":
		return r.leaderboardDavinci(ctx, limit, offset)
	case "acquire":
		return r.leaderboardAcquire(ctx, limit, offset)
	default:
		return nil, errors.New("invalid game_type")
	}
}

func (r *Repo) leaderboardDavinci(ctx context.Context, limit, offset int) ([]LeaderboardEntry, error) {
	type row struct {
		UserID     int64
		TotalGames int
		Wins       int
		WinRate    float64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("game_players AS gp").
		Joins("JOIN games g ON g.id = gp.game_id").
		Select(`gp.user_id AS user_id,
		        COUNT(*) AS total_games,
		        COUNT(*) FILTER (WHERE gp.is_winner) AS wins,
		        1.0 * COUNT(*) FILTER (WHERE gp.is_winner) / COUNT(*) AS win_rate`).
		Where("g.game_type = ? AND g.ended_at IS NOT NULL AND gp.user_id IS NOT NULL", "davinci").
		Group("gp.user_id").
		Order("win_rate DESC, total_games DESC, gp.user_id ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	entries := make([]LeaderboardEntry, 0, len(rows))
	userIDs := make([]int64, 0, len(rows))
	for _, rr := range rows {
		wins := rr.Wins
		winRate := rr.WinRate
		entries = append(entries, LeaderboardEntry{
			UserID:     rr.UserID,
			TotalGames: rr.TotalGames,
			Wins:       &wins,
			WinRate:    &winRate,
		})
		userIDs = append(userIDs, rr.UserID)
	}
	if err := r.fillLatestPlayerIDs(ctx, "davinci", userIDs, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *Repo) leaderboardAcquire(ctx context.Context, limit, offset int) ([]LeaderboardEntry, error) {
	type row struct {
		UserID     int64
		TotalGames int
		AvgRank    float64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("game_players AS gp").
		Joins("JOIN games g ON g.id = gp.game_id").
		Select(`gp.user_id AS user_id,
		        COUNT(*) AS total_games,
		        AVG(gp.final_rank)::float AS avg_rank`).
		Where("g.game_type = ? AND g.ended_at IS NOT NULL AND gp.user_id IS NOT NULL AND gp.final_rank IS NOT NULL", "acquire").
		Group("gp.user_id").
		Order("avg_rank ASC, total_games DESC, gp.user_id ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	entries := make([]LeaderboardEntry, 0, len(rows))
	userIDs := make([]int64, 0, len(rows))
	for _, rr := range rows {
		avgRank := rr.AvgRank
		entries = append(entries, LeaderboardEntry{
			UserID:     rr.UserID,
			TotalGames: rr.TotalGames,
			AvgRank:    &avgRank,
		})
		userIDs = append(userIDs, rr.UserID)
	}
	if err := r.fillLatestPlayerIDs(ctx, "acquire", userIDs, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// fillLatestPlayerIDs 批量取每个 user_id 在该 game_type 下最近一局使用的 player_id。
func (r *Repo) fillLatestPlayerIDs(ctx context.Context, gameType string, userIDs []int64, entries []LeaderboardEntry) error {
	if len(userIDs) == 0 {
		return nil
	}
	type pidRow struct {
		UserID   int64
		PlayerID string
	}
	var prs []pidRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (gp.user_id) gp.user_id AS user_id, gp.player_id AS player_id
		FROM game_players gp
		JOIN games g ON g.id = gp.game_id
		WHERE g.game_type = ? AND gp.user_id IN ?
		ORDER BY gp.user_id, g.started_at DESC
	`, gameType, userIDs).Scan(&prs).Error
	if err != nil {
		return err
	}
	pidByUser := make(map[int64]string, len(prs))
	for _, pr := range prs {
		pidByUser[pr.UserID] = pr.PlayerID
	}
	for i := range entries {
		entries[i].PlayerID = pidByUser[entries[i].UserID]
	}
	return nil
}
