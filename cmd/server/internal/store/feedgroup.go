package store

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"downlink/pkg/models"
)

// ListFeedGroups returns all feed groups
func (s *GormStore) ListFeedGroups() ([]models.FeedGroup, error) {
	var groups []models.FeedGroup

	// Query feed_groups table directly
	rows, err := s.db.Table("feed_groups").Order("sort_order ASC").Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to list feed groups: %w", err)
	}
	defer rows.Close()

	// Manually build the FeedGroup objects
	for rows.Next() {
		var group models.FeedGroup
		if err := s.db.ScanRows(rows, &group); err != nil {
			return nil, fmt.Errorf("failed to scan feed group: %w", err)
		}
		groups = append(groups, group)
	}

	return groups, nil
}

// GetFeedGroup returns a feed group by Id
func (s *GormStore) GetFeedGroup(id string) (models.FeedGroup, error) {
	var group models.FeedGroup

	// Query the feed group
	if err := s.db.Table("feed_groups").Where("id = ?", id).Scan(&group).Error; err != nil {
		return group, fmt.Errorf("failed to get feed group: %w", err)
	}

	// Preload feeds manually
	var feeds []models.Feed
	if err := s.db.Where("group_id = ?", id).Find(&feeds).Error; err != nil {
		log.WithError(err).Warn("Failed to load feeds for feed group")
		// Continue without feeds
	} else {
		group.Feeds = feeds
	}

	return group, nil
}

// StoreFeedGroup stores a feed group
func (s *GormStore) StoreFeedGroup(group models.FeedGroup) error {
	// Convert to a map to avoid type issues
	groupMap := map[string]interface{}{
		"id":         group.Id,
		"name":       group.Name,
		"icon":       group.Icon,
		"sort_order": group.SortOrder,
	}

	// Check if it exists
	var count int64
	if err := s.db.Table("feed_groups").Where("id = ?", group.Id).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check if feed group exists: %w", err)
	}

	// Save or create as appropriate
	if count > 0 {
		if err := s.db.Table("feed_groups").Where("id = ?", group.Id).Updates(groupMap).Error; err != nil {
			return fmt.Errorf("failed to update feed group: %w", err)
		}
	} else {
		if err := s.db.Table("feed_groups").Create(groupMap).Error; err != nil {
			return fmt.Errorf("failed to create feed group: %w", err)
		}
	}

	return nil
}

// DeleteFeedGroup deletes a feed group
func (s *GormStore) DeleteFeedGroup(id string) error {
	// Check if it's the default group
	if id == "default" {
		return fmt.Errorf("cannot delete the default feed group")
	}

	// Start a transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Move all feeds in this group to the default group in a single update
	if err := tx.Model(&models.Feed{}).Where("group_id = ?", id).Update("group_id", "default").Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to move feeds to default group: %w", err)
	}

	// Delete the group - use the table name instead of the model
	if err := tx.Table("feed_groups").Where("id = ?", id).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete feed group: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
