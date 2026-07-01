package manager

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateRefreshRunIdFormat(t *testing.T) {
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	id := generateRefreshRunId(ts)
	if !strings.HasPrefix(id, "refresh-") {
		t.Errorf("id = %q, want prefix \"refresh-\"", id)
	}
	// "refresh-" (8) + 12 hex chars = 20 total
	if len(id) != 20 {
		t.Errorf("id len = %d, want 20", len(id))
	}
}

func TestGenerateRefreshRunIdIsDeterministic(t *testing.T) {
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if generateRefreshRunId(ts) != generateRefreshRunId(ts) {
		t.Error("same time produced different ids")
	}
}

func TestGenerateRefreshRunIdDifferentTimesProduceDifferentIds(t *testing.T) {
	t1 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Nanosecond)
	if generateRefreshRunId(t1) == generateRefreshRunId(t2) {
		t.Error("nanosecond-apart times produced same id")
	}
}
