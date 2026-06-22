package store

import (
	"fmt"

	"github.com/ma111e/downlink/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListProfiles returns all profiles ordered by sort order, with their feeds
// preloaded.
func (s *GormStore) ListProfiles() ([]models.Profile, error) {
	var profiles []models.Profile
	if err := s.db.Preload("Feeds").Order("sort_order ASC").Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("failed to list profiles: %w", err)
	}
	return profiles, nil
}

// GetProfile returns a single profile by id (slug). Feeds are not preloaded
// (resolving editorial config does not need them); use ListProfileFeeds for the
// pool.
func (s *GormStore) GetProfile(id string) (models.Profile, error) {
	var profile models.Profile
	if err := s.db.First(&profile, "id = ?", id).Error; err != nil {
		return profile, fmt.Errorf("failed to get profile %q: %w", id, err)
	}
	return profile, nil
}

// StoreProfile upserts a profile row. Feed membership is managed separately via
// SetProfileFeeds, so the many-to-many association is omitted here.
func (s *GormStore) StoreProfile(profile models.Profile) error {
	if err := s.db.Omit("Feeds").Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&profile).Error; err != nil {
		return fmt.Errorf("failed to store profile %q: %w", profile.Id, err)
	}
	return nil
}

// DeleteProfile deletes a profile and its feed-membership rows. The default
// profile cannot be deleted.
func (s *GormStore) DeleteProfile(id string) error {
	if id == "default" {
		return fmt.Errorf("cannot delete the default profile")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM profile_feeds WHERE profile_id = ?", id).Error; err != nil {
			return fmt.Errorf("failed to clear profile feeds: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&models.Profile{}).Error; err != nil {
			return fmt.Errorf("failed to delete profile: %w", err)
		}
		return nil
	})
}

// SetProfileFeeds replaces a profile's feed membership with exactly feedIds.
func (s *GormStore) SetProfileFeeds(profileId string, feedIds []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM profile_feeds WHERE profile_id = ?", profileId).Error; err != nil {
			return fmt.Errorf("failed to clear profile feeds: %w", err)
		}
		for _, fid := range feedIds {
			if err := tx.Exec(
				"INSERT INTO profile_feeds (profile_id, feed_id) VALUES (?, ?)",
				profileId, fid,
			).Error; err != nil {
				return fmt.Errorf("failed to add feed %q to profile %q: %w", fid, profileId, err)
			}
		}
		return nil
	})
}

// ListProfileFeeds returns the feeds in a profile's pool.
func (s *GormStore) ListProfileFeeds(profileId string) ([]models.Feed, error) {
	var feeds []models.Feed
	if err := s.db.Where(
		"id IN (SELECT feed_id FROM profile_feeds WHERE profile_id = ?)", profileId,
	).Find(&feeds).Error; err != nil {
		return nil, fmt.Errorf("failed to list feeds for profile %q: %w", profileId, err)
	}
	return feeds, nil
}

// ListDigestsByProfile returns a profile's digests, newest first. See ListDigests
// for the meaning of full.
func (s *GormStore) ListDigestsByProfile(profileId string, limit int, full bool) ([]models.Digest, error) {
	return s.listDigests(profileId, limit, full)
}
