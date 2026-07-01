package store

import "testing"

func TestFreshDBUsesWALJournalMode(t *testing.T) {
	s := newTestStore(t)
	var mode string
	if err := s.db.Raw("PRAGMA journal_mode").Row().Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want \"wal\"", mode)
	}
}

func TestFreshDBEnablesForeignKeys(t *testing.T) {
	s := newTestStore(t)
	var enabled int
	if err := s.db.Raw("PRAGMA foreign_keys").Row().Scan(&enabled); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d, want 1 (ON)", enabled)
	}
}
