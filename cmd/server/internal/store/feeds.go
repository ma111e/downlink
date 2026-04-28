package store

import (
	"downlink/pkg/models"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

func (s *GormStore) StoreFeed(feed models.Feed) error {
	result := s.db.Save(&feed)
	if result.Error != nil {
		return fmt.Errorf("failed to store feed: %w", result.Error)
	}

	log.WithFields(log.Fields{
		"id":      feed.Id,
		"url":     feed.URL,
		"title":   feed.Title,
		"enabled": feed.Enabled,
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
	// due to ON DELETE CASCADE specified in the model relationships

	return nil
}
