package store

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func mkResult(id, digestId, provider, model string) models.DigestProviderResult {
	return models.DigestProviderResult{
		Id:           id,
		DigestId:     digestId,
		ProviderType: provider,
		ModelName:    model,
	}
}

func TestStoreAndGetDigestProviderResult(t *testing.T) {
	s := newTestStore(t)

	if err := s.StoreDigestProviderResult(mkResult("r1", "d1", "claude", "claude-opus")); err != nil {
		t.Fatalf("StoreDigestProviderResult: %v", err)
	}

	got, err := s.GetDigestProviderResult("r1")
	if err != nil {
		t.Fatalf("GetDigestProviderResult: %v", err)
	}
	if got.Id != "r1" || got.DigestId != "d1" || got.ProviderType != "claude" || got.ModelName != "claude-opus" {
		t.Errorf("got %+v", got)
	}
}

func TestGetDigestProviderResultsFiltersToDigest(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{"r2", "r3"} {
		if err := s.StoreDigestProviderResult(mkResult(id, "d2", "openai", "gpt-4")); err != nil {
			t.Fatalf("StoreDigestProviderResult(%s): %v", id, err)
		}
	}
	// Different digest — must not appear in results for d2.
	if err := s.StoreDigestProviderResult(mkResult("r4", "d9", "openai", "gpt-4")); err != nil {
		t.Fatalf("StoreDigestProviderResult(r4): %v", err)
	}

	got, err := s.GetDigestProviderResults("d2")
	if err != nil {
		t.Fatalf("GetDigestProviderResults: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
	for _, r := range got {
		if r.DigestId != "d2" {
			t.Errorf("unexpected DigestId = %q in results for d2", r.DigestId)
		}
	}
}

func TestGetDigestProviderResultNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetDigestProviderResult("ghost"); err == nil {
		t.Fatal("GetDigestProviderResult(ghost) error = nil, want not-found")
	}
}

func TestGetDigestProviderResultsUnknownDigestReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetDigestProviderResults("ghost")
	if err != nil {
		t.Fatalf("GetDigestProviderResults(ghost) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestStoreDigestProviderResultAutoAssignsId(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreDigestProviderResult(mkResult("", "d5", "claude", "haiku")); err != nil {
		t.Fatalf("StoreDigestProviderResult: %v", err)
	}
	results, err := s.GetDigestProviderResults("d5")
	if err != nil {
		t.Fatalf("GetDigestProviderResults: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Id == "" {
		t.Error("Id was not auto-assigned")
	}
}
