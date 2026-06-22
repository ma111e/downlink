package store

import (
	"fmt"
	"github.com/ma111e/downlink/pkg/models"
	"time"

	log "github.com/sirupsen/logrus"
)

func (s *GormStore) StoreFeed(feed models.Feed) error {
	result := s.db.Save(&feed)
	if result.Error != nil {
		return fmt.Errorf("failed to store feed: %w", result.Error)
	}

	enabled := false
	if feed.Enabled != nil {
		enabled = *feed.Enabled
	}
	log.WithFields(log.Fields{
		"id":      feed.Id,
		"url":     feed.URL,
		"title":   feed.Title,
		"enabled": enabled,
	}).Info("Feed stored successfully")

	return nil
}

func (s *GormStore) GetFeed(id string) (models.Feed, error) {
	var feed models.Feed
	result := s.db.First(&feed, "id = ?", id)
	if result.Error != nil {
		return feed, fmt.Errorf("failed to get feed: %w", result.Error)
	}
	return feed, nil
}

func (s *GormStore) ListFeeds() ([]models.Feed, error) {
	var feeds []models.Feed
	result := s.db.Find(&feeds)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list feeds: %w", result.Error)
	}

	// Populate each feed's topics in one batch query (one extra round-trip rather
	// than one per feed).
	if len(feeds) > 0 {
		var rows []models.FeedTopic
		if err := s.db.Order("topic ASC").Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to load feed topics: %w", err)
		}
		byFeed := make(map[string][]string, len(feeds))
		for _, r := range rows {
			byFeed[r.FeedId] = append(byFeed[r.FeedId], r.Topic)
		}
		for i := range feeds {
			feeds[i].Topics = byFeed[feeds[i].Id]
		}
	}
	return feeds, nil
}

func (s *GormStore) UpdateFeedLastFetch(id string, lastFetch time.Time) error {
	result := s.db.Model(&models.Feed{}).Where("id = ?", id).Update("last_fetch", lastFetch)
	if result.Error != nil {
		return fmt.Errorf("failed to update feed last_fetch: %w", result.Error)
	}
	return nil
}

func (s *GormStore) DeleteFeed(id string) error {
	result := s.db.Delete(&models.Feed{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete feed: %w", result.Error)
	}

	// Articles associated with this feed will be deleted automatically
	// due to ON DELETE CASCADE specified in the model relationships.
	// Topic and profile-membership rows have no FK cascade, so clear them here.
	if err := s.db.Where("feed_id = ?", id).Delete(&models.FeedTopic{}).Error; err != nil {
		return fmt.Errorf("failed to delete feed topics: %w", err)
	}
	if err := s.db.Exec("DELETE FROM profile_feeds WHERE feed_id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete feed profile memberships: %w", err)
	}

	return nil
}
