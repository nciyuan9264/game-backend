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
