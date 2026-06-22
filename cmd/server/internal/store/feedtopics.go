package store

import (
	"fmt"
	"strings"

	"github.com/ma111e/downlink/pkg/models"
	"gorm.io/gorm"
)

// SetFeedTopics replaces a feed's topic rows with exactly topics (lowercased,
// trimmed, de-duplicated). An empty list clears the feed's topics.
func (s *GormStore) SetFeedTopics(feedId string, topics []string) error {
	seen := make(map[string]struct{}, len(topics))
	var rows []models.FeedTopic
	for _, t := range topics {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		rows = append(rows, models.FeedTopic{FeedId: feedId, Topic: t})
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("feed_id = ?", feedId).Delete(&models.FeedTopic{}).Error; err != nil {
			return fmt.Errorf("clear feed topics: %w", err)
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return fmt.Errorf("insert feed topics: %w", err)
			}
		}
		return nil
	})
}

// ListFeedTopics returns a feed's topics.
func (s *GormStore) ListFeedTopics(feedId string) ([]string, error) {
	var topics []string
	if err := s.db.Model(&models.FeedTopic{}).
		Where("feed_id = ?", feedId).
		Order("topic ASC").
		Pluck("topic", &topics).Error; err != nil {
		return nil, fmt.Errorf("list feed topics: %w", err)
	}
	return topics, nil
}

// ListAllTopics returns the distinct topics in use across all feeds, sorted.
func (s *GormStore) ListAllTopics() ([]string, error) {
	var topics []string
	if err := s.db.Model(&models.FeedTopic{}).
		Distinct("topic").
		Order("topic ASC").
		Pluck("topic", &topics).Error; err != nil {
		return nil, fmt.Errorf("list all topics: %w", err)
	}
	return topics, nil
}

// FeedIDsByTopics returns the distinct ids of enabled feeds having any of the
// given topics. An empty topic list returns no feeds.
func (s *GormStore) FeedIDsByTopics(topics []string) ([]string, error) {
	if len(topics) == 0 {
		return nil, nil
	}
	norm := make([]string, 0, len(topics))
	for _, t := range topics {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			norm = append(norm, t)
		}
	}
	if len(norm) == 0 {
		return nil, nil
	}
	var ids []string
	if err := s.db.Model(&models.FeedTopic{}).
		Distinct("feed_topics.feed_id").
		Joins("JOIN feeds ON feeds.id = feed_topics.feed_id").
		Where("feed_topics.topic IN ? AND feeds.enabled = ?", norm, true).
		Pluck("feed_topics.feed_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("feed ids by topics: %w", err)
	}
	return ids, nil
}

// ListEnabledFeedIDs returns the ids of all enabled feeds.
func (s *GormStore) ListEnabledFeedIDs() ([]string, error) {
	var ids []string
	if err := s.db.Model(&models.Feed{}).
		Where("enabled = ?", true).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list enabled feed ids: %w", err)
	}
	return ids, nil
}
