package beat

import (
	"strings"
	"testing"
	"time"
)

func TestNewBeat(t *testing.T) {
	b := NewBeat("test content", Impetus{Label: "test label"})

	if b.Content != "test content" {
		t.Errorf("Content = %q, want %q", b.Content, "test content")
	}

	if b.Impetus.Label != "test label" {
		t.Errorf("Impetus.Label = %q, want %q", b.Impetus.Label, "test label")
	}

	if !strings.HasPrefix(b.ID, "beat-") {
		t.Errorf("ID = %q, want prefix 'beat-'", b.ID)
	}

	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	if b.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestGenerateID(t *testing.T) {
	now := time.Date(2025, 12, 11, 10, 30, 0, 0, time.UTC)
	id := GenerateID(now)

	expected := "beat-20251211-001"
	if id != expected {
		t.Errorf("GenerateID() = %q, want %q", id, expected)
	}
}

func TestGenerateIDWithSequence(t *testing.T) {
	now := time.Date(2025, 12, 11, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		seq  int
		want string
	}{
		{1, "beat-20251211-001"},
		{5, "beat-20251211-005"},
		{42, "beat-20251211-042"},
		{100, "beat-20251211-100"},
	}

	for _, tt := range tests {
		got := GenerateIDWithSequence(now, tt.seq)
		if got != tt.want {
			t.Errorf("GenerateIDWithSequence(%d) = %q, want %q", tt.seq, got, tt.want)
		}
	}
}

func TestProposedBeat_ToBeat(t *testing.T) {
	proposed := &ProposedBeat{
		Content:     "proposed content",
		Impetus:     Impetus{Label: "proposed"},
		LinkedBeads: []string{"bead-1", "bead-2"},
	}

	b := proposed.ToBeat(5)

	if b.Content != "proposed content" {
		t.Errorf("Content = %q, want %q", b.Content, "proposed content")
	}

	if len(b.LinkedBeads) != 2 {
		t.Errorf("LinkedBeads len = %d, want 2", len(b.LinkedBeads))
	}

	if !strings.Contains(b.ID, "-005") {
		t.Errorf("ID = %q, want sequence 005", b.ID)
	}
}

func TestProposedBeat_ToBeatWithCreatedAt(t *testing.T) {
	customTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	proposed := &ProposedBeat{
		Content:   "backdated content",
		Impetus:   Impetus{Label: "backdated"},
		CreatedAt: &customTime,
	}

	b := proposed.ToBeat(1)

	if !strings.Contains(b.ID, "20240115") {
		t.Errorf("ID = %q, want date 20240115", b.ID)
	}

	if !b.CreatedAt.Equal(customTime) {
		t.Errorf("CreatedAt = %v, want %v", b.CreatedAt, customTime)
	}
}
