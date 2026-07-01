package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestCategoryRoundTrip(t *testing.T) {
	in := &models.Category{Name: "security", Color: "#f00", Icon: "shield"}
	out := CategoryToModel(CategoryToProto(in))
	if out.Name != "security" || out.Color != "#f00" || out.Icon != "shield" {
		t.Errorf("round-trip lost: %+v", out)
	}
}

func TestAllCategoriesToProtoAndBack(t *testing.T) {
	in := []models.Category{
		{Name: "ai", Color: "#0f0", Icon: "robot"},
		{Name: "crypto", Color: "#00f", Icon: "lock"},
	}
	out := AllCategoriesToModels(AllCategoriesToProto(in))
	if len(out) != 2 || out[0].Name != "ai" || out[1].Name != "crypto" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
	if out[0].Color != "#0f0" || out[1].Icon != "lock" {
		t.Errorf("field values lost: %+v", out)
	}
}

func TestAllCategoriesToProtoEmptySlice(t *testing.T) {
	if got := AllCategoriesToProto(nil); got != nil {
		t.Errorf("AllCategoriesToProto(nil) = %v, want nil", got)
	}
}
