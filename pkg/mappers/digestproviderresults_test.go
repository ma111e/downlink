package mappers

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func TestDigestProviderResultRoundTripPreservesFields(t *testing.T) {
	created := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	in := &models.DigestProviderResult{
		Id:                     "r1",
		DigestId:               "d1",
		ProviderType:           "claude",
		ModelName:              "claude-opus",
		BriefOverview:          "brief",
		StandardSynthesis:      "standard",
		ComprehensiveSynthesis: "comprehensive",
		ProcessingTime:         1.23,
		Error:                  "",
		CreatedAt:              created,
	}
	out := DigestProviderResultToModel(DigestProviderResultToProto(in))
	if out == nil {
		t.Fatal("DigestProviderResultToModel returned nil")
	}
	if out.Id != "r1" || out.DigestId != "d1" || out.ProviderType != "claude" || out.ModelName != "claude-opus" {
		t.Errorf("id/digest/provider/model lost: %+v", out)
	}
	if out.BriefOverview != "brief" || out.StandardSynthesis != "standard" || out.ComprehensiveSynthesis != "comprehensive" {
		t.Errorf("text fields lost: %+v", out)
	}
	if out.ProcessingTime != 1.23 {
		t.Errorf("ProcessingTime = %v, want 1.23", out.ProcessingTime)
	}
	if !out.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", out.CreatedAt, created)
	}
}

func TestDigestProviderResultNilIsNil(t *testing.T) {
	if DigestProviderResultToProto(nil) != nil {
		t.Fatal("DigestProviderResultToProto(nil) != nil")
	}
	if DigestProviderResultToModel(nil) != nil {
		t.Fatal("DigestProviderResultToModel(nil) != nil")
	}
}

func TestDigestProviderResultErrorFieldPreserved(t *testing.T) {
	in := &models.DigestProviderResult{Id: "r2", Error: "timeout"}
	out := DigestProviderResultToModel(DigestProviderResultToProto(in))
	if out.Error != "timeout" {
		t.Errorf("Error = %q, want \"timeout\"", out.Error)
	}
}

func TestAllDigestProviderResultsRoundTrip(t *testing.T) {
	in := []models.DigestProviderResult{
		{Id: "r1", DigestId: "d1", ProviderType: "claude"},
		{Id: "r2", DigestId: "d1", ProviderType: "openai"},
	}
	out := AllDigestProviderResultsToModels(AllDigestProviderResultsToProto(in))
	if len(out) != 2 || out[0].Id != "r1" || out[1].Id != "r2" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}
