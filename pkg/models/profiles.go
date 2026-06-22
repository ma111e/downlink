package models

import (
	"encoding/json"

	"gorm.io/gorm"
)

// Profile is one curated, public view of the shared article pool. A profile
// selects a subset of feeds (many-to-many via profile_feeds, so pools may
// overlap between profiles), carries its own editorial config (how its articles
// are analyzed), and picks its own presentation (layout template set + default
// theme palette). Articles and feeds stay global; analyses and digests are
// scoped to a profile via their ProfileId.
type Profile struct {
	Id           string `gorm:"primaryKey" json:"id"` // slug, e.g. "infosec"
	Name         string `gorm:"uniqueIndex" json:"name"`
	Description  string `json:"description,omitempty"`
	Icon         string `json:"icon,omitempty"`
	Layout       string `json:"layout,omitempty"` // digestlayouts template set; empty = "default"
	Theme        string `json:"theme,omitempty"`  // digestthemes palette; empty = template default
	Enabled      *bool  `gorm:"default:true" json:"enabled"`
	SortOrder    *int   `gorm:"default:0" json:"sort_order"`
	OutputSubdir string `json:"output_subdir,omitempty"` // GitHub Pages subdir; empty = slug

	// Editorial is hydrated from EditorialJson on read and serialized back on
	// save (same JSON-column pattern as ArticleAnalysis). A nil field inside it
	// means "inherit the global/default behavior".
	EditorialJson string            `gorm:"column:editorial;type:text" json:"-"`
	Editorial     *ProfileEditorial `gorm:"-" json:"editorial,omitempty"`

	// Selection is the feed-membership rule (topics + explicit overrides),
	// persisted so feed-catalog changes can re-resolve profile_feeds without
	// re-reading profiles.yml. The resolved membership lives in profile_feeds.
	SelectionJson string            `gorm:"column:selection;type:text" json:"-"`
	Selection     *ProfileSelection `gorm:"-" json:"selection,omitempty"`

	Feeds []Feed `gorm:"many2many:profile_feeds;" json:"-"`
}

// ProfileSelection is how a profile chooses its feeds. Membership is the feeds
// whose topics intersect Topics, plus IncludeFeedIds, minus ExcludeFeedIds,
// restricted to enabled feeds. An empty selection (no topics, no includes) means
// "all enabled feeds". Include/exclude are stored as feed ids (resolved at apply
// time) so re-resolution does not need the original URLs.
type ProfileSelection struct {
	Topics         []string `json:"topics,omitempty"`
	IncludeFeedIds []string `json:"include_feed_ids,omitempty"`
	ExcludeFeedIds []string `json:"exclude_feed_ids,omitempty"`
}

// TableName specifies the table name for Profile.
func (Profile) TableName() string { return "profiles" }

// ProfileEditorial is a profile's editorial brain. Every field is optional; an
// empty/nil value inherits from the global AnalysisConfig + package defaults
// (resolved in services.ResolveEditorial). It mirrors AnalysisConfig and adds
// per-profile taxonomy, rubric, and raw prompt overrides.
type ProfileEditorial struct {
	Provider     string `json:"provider,omitempty" yaml:"provider,omitempty"`           // configured provider name
	Model        string `json:"model,omitempty" yaml:"model,omitempty"`                 // optional model override
	Persona      string `json:"persona,omitempty" yaml:"persona,omitempty"`             // analysis system-message prefix
	WritingStyle string `json:"writing_style,omitempty" yaml:"writing_style,omitempty"` // digest-summary style guide
	Audience     string `json:"audience,omitempty" yaml:"audience,omitempty"`          // target reader, injected into prompts

	Glossary               *bool `json:"glossary,omitempty" yaml:"glossary,omitempty"`
	VibeScore              *bool `json:"vibe_score,omitempty" yaml:"vibe_score,omitempty"` // legacy single-number scoring instead of the rubric
	StandardSynthesis      *bool `json:"standard_synthesis,omitempty" yaml:"standard_synthesis,omitempty"`
	ComprehensiveSynthesis *bool `json:"comprehensive_synthesis,omitempty" yaml:"comprehensive_synthesis,omitempty"`
	ExecutiveSummary       *bool `json:"executive_summary,omitempty" yaml:"executive_summary,omitempty"`

	Categories []CategoryDef    `json:"categories,omitempty" yaml:"categories,omitempty"` // nil/empty = default category set
	Rubric     *RubricConfig    `json:"rubric,omitempty" yaml:"rubric,omitempty"`         // nil = default weights/thresholds
	Prompts    *PromptOverrides `json:"prompts,omitempty" yaml:"prompts,omitempty"`       // nil = built-in task instructions
}

// CategoryDef is one allowed category for a profile, with a short description the
// LLM is shown when classifying.
type CategoryDef struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// RubricConfig overrides the importance model for a profile. Weights keys are the
// six rubric dimensions (specificity, severity, breadth, novelty, actionability,
// credibility); nil sub-fields fall back to scoring.DefaultConfig().
type RubricConfig struct {
	Weights         map[string]float64 `json:"weights,omitempty" yaml:"weights,omitempty"`
	Tiers           *TierThresholds    `json:"tiers,omitempty" yaml:"tiers,omitempty"`
	AggregatorScore *int               `json:"aggregator_score,omitempty" yaml:"aggregator_score,omitempty"`
	EvergreenCap    *int               `json:"evergreen_cap,omitempty" yaml:"evergreen_cap,omitempty"`
	PromoCap        *int               `json:"promo_cap,omitempty" yaml:"promo_cap,omitempty"`
}

// TierThresholds are the inclusive lower bounds for the read tiers on a 0-100 score.
type TierThresholds struct {
	Must   int `json:"must" yaml:"must"`
	Should int `json:"should" yaml:"should"`
	May    int `json:"may" yaml:"may"`
}

// PromptOverrides lets a profile replace task instructions verbatim. Tasks is
// keyed by analysis task name (categorize, tldr, plain_words, key_points,
// insights, referenced_reports, summaries, glossary, importance). Output schema
// and required keys are NOT overridable, so validation still applies.
type PromptOverrides struct {
	Tasks         map[string]string `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	DigestSummary string            `json:"digest_summary,omitempty" yaml:"digest_summary,omitempty"`
	Dedupe        string            `json:"dedupe,omitempty" yaml:"dedupe,omitempty"`
}

// ProfilesFile is the YAML catalog of profiles (profiles.yml), mirroring
// FeedsFile. It is applied to the database at server startup: each entry is
// upserted and its feed pool (referenced by URL) is reconciled.
type ProfilesFile struct {
	Profiles []ProfileConfig `yaml:"profiles"`
}

// ProfileConfig is one profile definition in profiles.yml. Feeds are referenced
// by URL and resolved to feed ids at apply time.
type ProfileConfig struct {
	Slug         string            `yaml:"slug"`
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description,omitempty"`
	Icon         string            `yaml:"icon,omitempty"`
	Layout       string            `yaml:"layout,omitempty"`
	Theme        string            `yaml:"theme,omitempty"`
	Enabled      *bool             `yaml:"enabled,omitempty"`
	SortOrder    *int              `yaml:"sort_order,omitempty"`
	OutputSubdir string            `yaml:"output_subdir,omitempty"`
	Topics       []string          `yaml:"topics,omitempty"`        // feeds with ANY of these topics
	Feeds        []string          `yaml:"feeds,omitempty"`         // explicit include feed URLs
	ExcludeFeeds []string          `yaml:"exclude_feeds,omitempty"` // explicit exclude feed URLs
	Editorial    *ProfileEditorial `yaml:"editorial,omitempty"`
}

// BeforeSave serializes Editorial and Selection into their JSON columns before
// persisting.
func (p *Profile) BeforeSave(_ *gorm.DB) error {
	if p.Editorial != nil {
		b, err := json.Marshal(p.Editorial)
		if err != nil {
			return err
		}
		p.EditorialJson = string(b)
	}
	if p.Selection != nil {
		b, err := json.Marshal(p.Selection)
		if err != nil {
			return err
		}
		p.SelectionJson = string(b)
	}
	return nil
}

// AfterFind hydrates Editorial and Selection from their JSON columns after a query.
func (p *Profile) AfterFind(_ *gorm.DB) error {
	if p.EditorialJson != "" {
		var ed ProfileEditorial
		if err := json.Unmarshal([]byte(p.EditorialJson), &ed); err != nil {
			return err
		}
		p.Editorial = &ed
	}
	if p.SelectionJson != "" {
		var sel ProfileSelection
		if err := json.Unmarshal([]byte(p.SelectionJson), &sel); err != nil {
			return err
		}
		p.Selection = &sel
	}
	return nil
}
