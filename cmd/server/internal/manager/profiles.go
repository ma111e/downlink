package manager

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/notification"
	"github.com/ma111e/downlink/pkg/digestlayouts"
	"github.com/ma111e/downlink/pkg/digestthemes"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ProfileApplyResult summarizes a profiles.yml apply.
type ProfileApplyResult struct {
	Upserted []string // profile slugs created or updated
	Skipped  []string // profile slugs skipped (e.g. invalid)
	Warnings []string // soft config problems, already logged; kept for CLI display
}

// defaultProfileSlug is the always-present profile seeded by the store.
const defaultProfileSlug = "default"

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

	// Slug and publish-subdir collisions are hard errors: two profiles writing
	// into the same GitHub Pages subdirectory would overwrite each other's site.
	// The stored default profile participates too (unless the file redefines it).
	seenSlugs := make(map[string]bool)
	subdirOwner := make(map[string]string) // resolved subdir -> slug
	fileHasDefault := false
	for _, pc := range file.Profiles {
		if pc.Slug == defaultProfileSlug {
			fileHasDefault = true
		}
	}
	if !fileHasDefault {
		if def, err := m.store.GetProfile(defaultProfileSlug); err == nil {
			subdirOwner[configuredProfileSubdir(def.Id, def.OutputSubdir)] = def.Id
		}
	}
	for _, pc := range file.Profiles {
		if pc.Slug == "" {
			continue
		}
		if seenSlugs[pc.Slug] {
			return result, fmt.Errorf("duplicate profile slug %q in profiles.yml", pc.Slug)
		}
		seenSlugs[pc.Slug] = true
		sub := configuredProfileSubdir(pc.Slug, pc.OutputSubdir)
		if owner, taken := subdirOwner[sub]; taken {
			return result, fmt.Errorf("profiles %q and %q both publish to output subdir %q", owner, pc.Slug, sub)
		}
		subdirOwner[sub] = pc.Slug
	}

	for _, pc := range file.Profiles {
		if pc.Slug == "" {
			log.Warn("skipping profile with empty slug in profiles.yml")
			result.Skipped = append(result.Skipped, "(empty slug)")
			continue
		}

		for _, w := range validateProfileConfig(pc) {
			log.WithField("profile", pc.Slug).Warn(w)
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", pc.Slug, w))
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

// rubricWeightKeys are the six scoring dimensions a profile rubric may weight;
// anything else in the weights map is silently ignored by the score overlay.
var rubricWeightKeys = map[string]bool{
	"specificity":   true,
	"severity":      true,
	"breadth":       true,
	"novelty":       true,
	"actionability": true,
	"credibility":   true,
}

// validateProfileConfig checks one profiles.yml entry for soft problems that
// would otherwise misbehave silently (typoed prompt/weight keys, off-sum
// weights, misordered tiers) or fail only later at publish time (unknown
// layout/theme). Returns human-readable warnings; an empty slice means clean.
func validateProfileConfig(pc models.ProfileConfig) []string {
	var warnings []string
	warnf := func(format string, args ...interface{}) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	if pc.Layout != "" && !digestlayouts.Valid(pc.Layout) && !notification.OnDiskLayoutExists(pc.Layout) {
		warnf("unknown layout %q (not a built-in layout and not found in the layouts dir); the default layout will be used", pc.Layout)
	}
	if pc.Theme != "" && !digestthemes.Valid(pc.Theme) {
		warnf("unknown theme %q; the template default will be used", pc.Theme)
	}

	ed := pc.Editorial
	if ed == nil {
		return warnings
	}

	// Effective scoring mode mirrors ResolveEditorial: the profile's vibe_score
	// wins, otherwise the global analysis config decides.
	vibe := false
	if config.Config != nil {
		vibe = config.Config.Analysis.VibeScore
	}
	if ed.VibeScore != nil {
		vibe = *ed.VibeScore
	}

	if ed.Prompts != nil {
		keys := make([]string, 0, len(ed.Prompts.Tasks))
		for k := range ed.Prompts.Tasks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch {
			case !models.KnownPromptTaskNames[k]:
				valid := make([]string, 0, len(models.KnownPromptTaskNames))
				for name := range models.KnownPromptTaskNames {
					valid = append(valid, name)
				}
				sort.Strings(valid)
				warnf("unknown prompt task %q; valid tasks: %s", k, strings.Join(valid, ", "))
			case k == "importance" && !vibe:
				warnf("prompt task \"importance\" only runs with vibe_score: true; the default scoring task is \"rubric\"")
			case k == "rubric" && vibe:
				warnf("prompt task \"rubric\" does not run with vibe_score: true; the scoring task in that mode is \"importance\"")
			}
		}
	}

	if r := ed.Rubric; r != nil {
		if len(r.Weights) > 0 {
			// Overlay onto the defaults exactly like services.applyRubric, then
			// check the effective sum: Compute assumes weights sum to ~1, so an
			// off-sum config silently compresses or clips the 0-100 scale.
			w := scoring.DefaultConfig().Weights
			for k, v := range r.Weights {
				if !rubricWeightKeys[k] {
					warnf("unknown rubric weight %q (valid: specificity, severity, breadth, novelty, actionability, credibility); it is ignored", k)
					continue
				}
				switch k {
				case "specificity":
					w.Specificity = v
				case "severity":
					w.Severity = v
				case "breadth":
					w.Breadth = v
				case "novelty":
					w.Novelty = v
				case "actionability":
					w.Actionability = v
				case "credibility":
					w.Credibility = v
				}
			}
			sum := w.Specificity + w.Severity + w.Breadth + w.Novelty + w.Actionability + w.Credibility
			if math.Abs(sum-1.0) > 0.01 {
				warnf("rubric weights sum to %.2f (expected ~1.0); importance scores will be scaled accordingly", sum)
			}
		}
		if t := r.Tiers; t != nil {
			if t.Must > 100 || t.May < 0 || t.Must < t.Should || t.Should < t.May {
				warnf("rubric tiers {must: %d, should: %d, may: %d} are not ordered 100 >= must >= should >= may >= 0", t.Must, t.Should, t.May)
			}
		}
	}

	return warnings
}

// configuredProfileSubdir resolves the GitHub Pages subdirectory a profile
// publishes into, mirroring services.profileSubdir: the explicit output_subdir,
// else the global output dir for the default profile, else the slug, with
// "digests" as the final fallback.
func configuredProfileSubdir(slug, outputSubdir string) string {
	sub := outputSubdir
	if sub == "" {
		if slug == defaultProfileSlug {
			if config.Config != nil {
				sub = config.Config.Notifications.GitHubPages.OutputDir
			}
		} else {
			sub = slug
		}
	}
	if sub == "" {
		sub = "digests"
	}
	return sub
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
