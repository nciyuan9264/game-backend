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
