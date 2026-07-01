package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestFeedGroupRoundTripNoFeeds(t *testing.T) {
	sortOrder := 3
	in := &models.FeedGroup{
		Id:        "g1",
		Name:      "Security",
		Icon:      "shield",
		SortOrder: &sortOrder,
	}
	proto, err := FeedGroupToProto(in)
	if err != nil {
		t.Fatalf("FeedGroupToProto error = %v", err)
	}
	out, err := FeedGroupToModel(proto)
	if err != nil {
		t.Fatalf("FeedGroupToModel error = %v", err)
	}
	if out == nil {
		t.Fatal("FeedGroupToModel returned nil")
	}
	if out.Id != "g1" || out.Name != "Security" || out.Icon != "shield" {
		t.Errorf("id/name/icon lost: %+v", out)
	}
	if out.SortOrder == nil || *out.SortOrder != 3 {
		t.Errorf("SortOrder = %v, want 3", out.SortOrder)
	}
}

func TestFeedGroupToProtoNilSortOrderDefaultsToZero(t *testing.T) {
	in := &models.FeedGroup{Id: "g2", Name: "Default"}
	proto, err := FeedGroupToProto(in)
	if err != nil {
		t.Fatalf("FeedGroupToProto error = %v", err)
	}
	if proto.SortOrder != 0 {
		t.Errorf("SortOrder = %d, want 0 when nil", proto.SortOrder)
	}
}

func TestFeedGroupToProtoNilIsNil(t *testing.T) {
	proto, err := FeedGroupToProto(nil)
	if err != nil || proto != nil {
		t.Errorf("FeedGroupToProto(nil) = %v, %v; want nil, nil", proto, err)
	}
}

func TestFeedGroupToModelNilIsNil(t *testing.T) {
	out, err := FeedGroupToModel(nil)
	if err != nil || out != nil {
		t.Errorf("FeedGroupToModel(nil) = %v, %v; want nil, nil", out, err)
	}
}

func TestAllFeedGroupsRoundTripNoFeeds(t *testing.T) {
	so := 0
	in := []models.FeedGroup{
		{Id: "g1", Name: "Group 1", SortOrder: &so},
		{Id: "g2", Name: "Group 2", SortOrder: &so},
	}
	protos, err := AllFeedGroupsToProto(in)
	if err != nil {
		t.Fatalf("AllFeedGroupsToProto error = %v", err)
	}
	out, err := AllFeedGroupsToModels(protos)
	if err != nil {
		t.Fatalf("AllFeedGroupsToModels error = %v", err)
	}
	if len(out) != 2 || out[0].Id != "g1" || out[1].Id != "g2" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}
