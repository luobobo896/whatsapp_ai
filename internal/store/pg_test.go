package store

import (
	"slices"
	"testing"
)

func TestSplitQueryAddsBigramsWithoutSingletonNoise(t *testing.T) {
	got := splitQuery("羽绒服怎么洗？")
	want := []string{"羽绒服怎么洗", "羽绒", "绒服", "服怎", "怎么", "么洗"}
	if !slices.Equal(got, want) {
		t.Fatalf("splitQuery() = %#v, want %#v", got, want)
	}
}

func TestDailyReplyLimitReached(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		count int
		want  bool
	}{
		{name: "unlimited", limit: 0, count: 10000, want: false},
		{name: "below limit", limit: 3, count: 2, want: false},
		{name: "at limit", limit: 3, count: 3, want: true},
		{name: "over limit", limit: 3, count: 4, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dailyReplyLimitReached(tt.limit, tt.count); got != tt.want {
				t.Fatalf("dailyReplyLimitReached(%d, %d) = %t, want %t", tt.limit, tt.count, got, tt.want)
			}
		})
	}
}
