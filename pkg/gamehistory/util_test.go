package gamehistory

import "testing"

func TestEndReasonConstants(t *testing.T) {
	if EndReasonCompleted != "completed" {
		t.Fatalf("EndReasonCompleted = %q, want completed", EndReasonCompleted)
	}
	if EndReasonAbandoned != "abandoned" {
		t.Fatalf("EndReasonAbandoned = %q, want abandoned", EndReasonAbandoned)
	}
}

func TestAcquireEndVoteIsRecordable(t *testing.T) {
	if !IsRecordableCmd("game_end_vote") {
		t.Fatal("game_end_vote must be retained for replay")
	}
}

func TestLeaderboardSortOrders(t *testing.T) {
	if got := leaderboardAvgRankOrder(LeaderboardSortMetric); got != "avg_rank ASC, total_games DESC, gp.user_id ASC" {
		t.Fatalf("avg rank metric order = %q", got)
	}
	if got := leaderboardAvgRankOrder(LeaderboardSortTotalGames); got != "total_games DESC, avg_rank ASC, gp.user_id ASC" {
		t.Fatalf("avg rank total games order = %q", got)
	}
	if got := leaderboardDavinciOrder(LeaderboardSortMetric); got != "win_rate DESC, total_games DESC, gp.user_id ASC" {
		t.Fatalf("davinci metric order = %q", got)
	}
	if got := leaderboardDavinciOrder(LeaderboardSortTotalGames); got != "total_games DESC, win_rate DESC, gp.user_id ASC" {
		t.Fatalf("davinci total games order = %q", got)
	}
}

func TestParseLeaderboardSort(t *testing.T) {
	if sort, ok := ParseLeaderboardSort(""); !ok || sort != LeaderboardSortMetric {
		t.Fatalf("empty sort = %q, %v; want metric, true", sort, ok)
	}
	if sort, ok := ParseLeaderboardSort("totalGames"); !ok || sort != LeaderboardSortTotalGames {
		t.Fatalf("totalGames sort = %q, %v; want totalGames, true", sort, ok)
	}
	if _, ok := ParseLeaderboardSort("score"); ok {
		t.Fatal("score sort should be invalid")
	}
}
