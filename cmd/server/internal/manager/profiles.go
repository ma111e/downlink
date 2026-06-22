package manager

import (
	"fmt"
	"os"
	"strings"

	"github.com/ma111e/downlink/pkg/models"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ProfileApplyResult summarizes a profiles.yml apply.
type ProfileApplyResult struct {
	Upserted []string // profile slugs created or updated
	Skipped  []string // profile slugs skipped (e.g. invalid)
}

// ApplyProfiles reconciles the stored profiles against profiles.yml: each entry
// is upserted and its feed pool (referenced by URL) is set to exactly the
// resolved feed ids. Feeds are resolved the same way the feed catalog stores
// them (by domain id), so a profile picks up whichever feed serves that URL.
// Profiles absent from the file are left untouched (the default profile, in
// particular, is never removed here).
func (m *FeedManager) ApplyProfiles(file *models.ProfilesFile) (ProfileApplyResult, error) {
	var result ProfileApplyResult
	if file == nil {
		return result, nil
	}

	for _, pc := range file.Profiles {
		if pc.Slug == "" {
			log.Warn("skipping profile with empty slug in profiles.yml")
			result.Skipped = append(result.Skipped, "(empty slug)")
			continue
		}

		sel := buildProfileSelection(pc)
		profile := models.Profile{
			Id:           pc.Slug,
			Name:         pc.Name,
			Description:  pc.Description,
			Icon:         pc.Icon,
			Layout:       pc.Layout,
			Theme:        pc.Theme,
			Enabled:      pc.Enabled,
			SortOrder:    pc.SortOrder,
			OutputSubdir: pc.OutputSubdir,
			Editorial:    pc.Editorial,
			Selection:    sel,
		}
		if profile.Name == "" {
			profile.Name = pc.Slug
		}
		if err := m.store.StoreProfile(profile); err != nil {
			return result, fmt.Errorf("failed to store profile %q: %w", pc.Slug, err)
		}

		feedIDs, err := m.resolveProfileFeedIDs(sel)
		if err != nil {
			return result, fmt.Errorf("failed to resolve feeds for profile %q: %w", pc.Slug, err)
		}
		if err := m.store.SetProfileFeeds(pc.Slug, feedIDs); err != nil {
			return result, fmt.Errorf("failed to set feeds for profile %q: %w", pc.Slug, err)
		}

		result.Upserted = append(result.Upserted, pc.Slug)
	}

	return result, nil
}

// buildProfileSelection turns a profiles.yml entry into a stored selection rule:
// topics are lowercased, and the include/exclude feed URLs are resolved to feed
// ids (the same domain ids the catalog stores).
func buildProfileSelection(pc models.ProfileConfig) *models.ProfileSelection {
	sel := &models.ProfileSelection{}
	for _, t := range pc.Topics {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			sel.Topics = append(sel.Topics, t)
		}
	}
	resolve := func(urls []string) []string {
		var ids []string
		for _, url := range urls {
			id, err := generateFeedId(url)
			if err != nil {
				log.WithError(err).WithField("url", url).Warn("profile selection: skipping unresolvable feed URL")
				continue
			}
			ids = append(ids, id)
		}
		return ids
	}
	sel.IncludeFeedIds = resolve(pc.Feeds)
	sel.ExcludeFeedIds = resolve(pc.ExcludeFeeds)
	return sel
}

// resolveProfileFeedIDs materializes a selection rule into the set of enabled
// feed ids: feeds whose topics intersect sel.Topics, plus the explicit includes,
// minus the explicit excludes, all restricted to enabled feeds. A selection with
// no topics and no includes means "all enabled feeds" (then minus any excludes).
func (m *FeedManager) resolveProfileFeedIDs(sel *models.ProfileSelection) ([]string, error) {
	enabled, err := m.store.ListEnabledFeedIDs()
	if err != nil {
		return nil, err
	}
	enabledSet := make(map[string]struct{}, len(enabled))
	for _, id := range enabled {
		enabledSet[id] = struct{}{}
	}

	set := make(map[string]struct{})
	unscoped := sel == nil || (len(sel.Topics) == 0 && len(sel.IncludeFeedIds) == 0)
	if unscoped {
		for _, id := range enabled {
			set[id] = struct{}{}
		}
	} else {
		if len(sel.Topics) > 0 {
			ids, err := m.store.FeedIDsByTopics(sel.Topics) // already enabled-only
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				set[id] = struct{}{}
			}
		}
		for _, id := range sel.IncludeFeedIds {
			if _, ok := enabledSet[id]; ok {
				set[id] = struct{}{}
			}
		}
	}
	if sel != nil {
		for _, id := range sel.ExcludeFeedIds {
			delete(set, id)
		}
	}

	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out, nil
}

// RecomputeProfileFeeds re-resolves every profile's feed membership from its
// stored selection rule. Call it after the feed catalog changes (e.g. dlk feeds
// apply) so new feeds flow into the profiles whose topics they match.
func (m *FeedManager) RecomputeProfileFeeds() error {
	profiles, err := m.store.ListProfiles()
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	for _, p := range profiles {
		ids, err := m.resolveProfileFeedIDs(p.Selection)
		if err != nil {
			return fmt.Errorf("resolve feeds for profile %q: %w", p.Id, err)
		}
		if err := m.store.SetProfileFeeds(p.Id, ids); err != nil {
			return fmt.Errorf("set feeds for profile %q: %w", p.Id, err)
		}
	}
	return nil
}

// LoadProfilesFile parses a profiles.yml catalog from disk.
func LoadProfilesFile(path string) (*models.ProfilesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profiles file: %w", err)
	}
	var pf models.ProfilesFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &pf, nil
}
