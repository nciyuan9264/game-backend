package service

import "testing"

func TestParseSnapshotPageParams(t *testing.T) {
	tests := []struct {
		name        string
		offsetValue string
		limitValue  string
		wantOffset  int
		wantLimit   int
		wantError   bool
	}{
		{name: "defaults", wantOffset: 0, wantLimit: 20},
		{name: "explicit", offsetValue: "40", limitValue: "50", wantOffset: 40, wantLimit: 50},
		{name: "negative offset", offsetValue: "-1", wantError: true},
		{name: "invalid offset", offsetValue: "no", wantError: true},
		{name: "zero limit", limitValue: "0", wantError: true},
		{name: "negative limit", limitValue: "-1", wantError: true},
		{name: "limit above maximum", limitValue: "51", wantError: true},
		{name: "invalid limit", limitValue: "no", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, limit, err := parseSnapshotPageParams(tt.offsetValue, tt.limitValue)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError %v", err, tt.wantError)
			}
			if err == nil && (offset != tt.wantOffset || limit != tt.wantLimit) {
				t.Fatalf("got offset=%d limit=%d, want offset=%d limit=%d", offset, limit, tt.wantOffset, tt.wantLimit)
			}
		})
	}
}

func TestSnapshotEventWindow(t *testing.T) {
	tests := []struct {
		offset          int
		limit           int
		wantEventOffset int
		wantEventLimit  int
	}{
		{offset: 0, limit: 1, wantEventOffset: 0, wantEventLimit: 0},
		{offset: 0, limit: 20, wantEventOffset: 0, wantEventLimit: 19},
		{offset: 20, limit: 20, wantEventOffset: 19, wantEventLimit: 20},
	}
	for _, tt := range tests {
		eventOffset, eventLimit := snapshotEventWindow(tt.offset, tt.limit)
		if eventOffset != tt.wantEventOffset || eventLimit != tt.wantEventLimit {
			t.Fatalf("offset=%d limit=%d got (%d,%d), want (%d,%d)",
				tt.offset, tt.limit, eventOffset, eventLimit, tt.wantEventOffset, tt.wantEventLimit)
		}
	}
}

func TestSnapshotPageProgress(t *testing.T) {
	tests := []struct {
		name           string
		totalSnapshots int
		offset         int
		returned       int
		wantNext       int
		wantMore       bool
	}{
		{name: "first page", totalSnapshots: 121, offset: 0, returned: 20, wantNext: 20, wantMore: true},
		{name: "last short page", totalSnapshots: 45, offset: 40, returned: 5, wantNext: 45, wantMore: false},
		{name: "out of range", totalSnapshots: 45, offset: 60, returned: 0, wantNext: 45, wantMore: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, more := snapshotPageProgress(tt.totalSnapshots, tt.offset, tt.returned)
			if next != tt.wantNext || more != tt.wantMore {
				t.Fatalf("got next=%d more=%v, want next=%d more=%v", next, more, tt.wantNext, tt.wantMore)
			}
		})
	}
}
