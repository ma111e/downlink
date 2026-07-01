package store

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func sortPtr(n int) *int { return &n }

// findGroup returns the group with the given id, or nil. A "default" group is
// seeded by migration, so tests must not assume the list is empty.
func findGroup(groups []models.FeedGroup, id string) *models.FeedGroup {
	for i := range groups {
		if groups[i].Id == id {
			return &groups[i]
		}
	}
	return nil
}

func groupIndex(groups []models.FeedGroup, id string) int {
	for i := range groups {
		if groups[i].Id == id {
			return i
		}
	}
	return -1
}

func TestStoreFeedGroupCreateThenUpdate(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "g1", Name: "News", Icon: "paper", SortOrder: sortPtr(1)}); err != nil {
		t.Fatalf("StoreFeedGroup() create error = %v", err)
	}
	// Upsert on the same id must update, not duplicate.
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "g1", Name: "Updated", Icon: "paper", SortOrder: sortPtr(1)}); err != nil {
		t.Fatalf("StoreFeedGroup() update error = %v", err)
	}

	groups, err := s.ListFeedGroups()
	if err != nil {
		t.Fatalf("ListFeedGroups() error = %v", err)
	}
	// Count only g1 rows to prove the upsert did not duplicate it.
	g1Count := 0
	for _, g := range groups {
		if g.Id == "g1" {
			g1Count++
		}
	}
	if g1Count != 1 {
		t.Fatalf("g1 row count = %d, want 1 (upsert, not duplicate)", g1Count)
	}
	if g := findGroup(groups, "g1"); g == nil || g.Name != "Updated" {
		t.Fatalf("g1 = %v, want name Updated", g)
	}
}

func TestListFeedGroupsOrderedBySortOrder(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "b", Name: "B", SortOrder: sortPtr(2)}); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "a", Name: "A", SortOrder: sortPtr(1)}); err != nil {
		t.Fatal(err)
	}

	groups, err := s.ListFeedGroups()
	if err != nil {
		t.Fatalf("ListFeedGroups() error = %v", err)
	}
	// a (sort 1) must come before b (sort 2), regardless of the seeded default.
	ia, ib := groupIndex(groups, "a"), groupIndex(groups, "b")
	if ia == -1 || ib == -1 || ia >= ib {
		t.Fatalf("indices a=%d b=%d, want a before b by sort_order ASC", ia, ib)
	}
}

func TestGetFeedGroupPreloadsFeeds(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "g1", Name: "News", SortOrder: sortPtr(0)}); err != nil {
		t.Fatal(err)
	}
	gid := "g1"
	if err := s.StoreFeed(models.Feed{Id: "f1", GroupId: &gid}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}

	group, err := s.GetFeedGroup("g1")
	if err != nil {
		t.Fatalf("GetFeedGroup() error = %v", err)
	}
	if len(group.Feeds) != 1 || group.Feeds[0].Id != "f1" {
		t.Fatalf("group feeds = %v, want [f1]", group.Feeds)
	}
}

func TestDeleteFeedGroupRejectsDefault(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteFeedGroup("default"); err == nil {
		t.Fatal("DeleteFeedGroup(default) = nil, want protection error")
	}
}

func TestDeleteFeedGroupMovesFeedsToDefault(t *testing.T) {
	s := newTestStore(t)
	if err := s.StoreFeedGroup(models.FeedGroup{Id: "g1", Name: "News", SortOrder: sortPtr(0)}); err != nil {
		t.Fatal(err)
	}
	gid := "g1"
	if err := s.StoreFeed(models.Feed{Id: "f1", GroupId: &gid}); err != nil {
		t.Fatalf("StoreFeed() error = %v", err)
	}

	if err := s.DeleteFeedGroup("g1"); err != nil {
		t.Fatalf("DeleteFeedGroup() error = %v", err)
	}

	// g1 gone; the seeded default group remains.
	groups, _ := s.ListFeedGroups()
	if findGroup(groups, "g1") != nil {
		t.Fatal("g1 still present after DeleteFeedGroup")
	}
	if findGroup(groups, "default") == nil {
		t.Fatal("default group was removed, want it preserved")
	}
	// Feed reassigned to "default", not deleted.
	feed, err := s.GetFeed("f1")
	if err != nil {
		t.Fatalf("GetFeed() error = %v (feed should survive group deletion)", err)
	}
	if feed.GroupId == nil || *feed.GroupId != "default" {
		t.Fatalf("feed GroupId = %v, want default", feed.GroupId)
	}
}
