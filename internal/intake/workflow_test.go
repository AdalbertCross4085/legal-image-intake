package intake

import (
	"testing"
	"time"
)

func TestDecideDeadline(t *testing.T) {
	now := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		deadline time.Time
		want     Decision
	}{
		{"routine intake", now.Add(7 * 24 * time.Hour), Decision{Status: "document_ready", FollowUp: false}},
		{"deadline in three days", now.Add(3 * 24 * time.Hour), Decision{Status: "deadline_follow_up", FollowUp: true}},
		{"overdue matter", now.Add(-24 * time.Hour), Decision{Status: "deadline_follow_up", FollowUp: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideDeadline(now, tt.deadline); got != tt.want {
				t.Fatalf("DecideDeadline() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
