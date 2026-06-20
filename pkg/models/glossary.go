package models

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GlossaryKind distinguishes the two provenance sources of a glossary entry.
type GlossaryKind string

const (
	GlossaryKindJargon GlossaryKind = "jargon" // from ArticleAnalysis.GlossaryTerms
	GlossaryKindEntity GlossaryKind = "entity" // from a Tag (threat actor, malware, CVE, …)
)

// glossaryCategories is the fixed semantic taxonomy a glossary entry can be typed with,
// so entries can be handled differently by type downstream. The LLM picks one; anything
// outside this set (or empty) is coerced to "other".
var glossaryCategories = map[string]bool{
	"threat-actor":  true,
	"malware":       true,
	"tool":          true,
	"technique":     true,
	"vulnerability": true,
	"protocol":      true,
	"concept":       true,
	"organization":  true,
	"product":       true,
	"other":         true,
}

// GlossaryCategoryOther is the fallback category for unknown/empty values.
const GlossaryCategoryOther = "other"

// NormalizeGlossaryCategory lowercases/trims the value and returns it if it is in the
// fixed taxonomy, otherwise "other".
func NormalizeGlossaryCategory(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if glossaryCategories[s] {
		return s
	}
	return GlossaryCategoryOther
}

// GlossaryEntry is one deduplicated term in the persistent global glossary. Identity is the
// normalized key; definitions are reused across digests and never re-queried once present.
// A manual override, once set, wins forever and is never overwritten by regeneration.
type GlossaryEntry struct {
	Id                string       `gorm:"primaryKey" json:"id"`
	NormalizedKey     string       `gorm:"uniqueIndex" json:"normalized_key"`   // dedup identity
	Term              string       `json:"term"`                                // display form (first-seen)
	Kind              GlossaryKind `gorm:"index" json:"kind"`                   // provenance: jargon vs entity
	Category          string       `gorm:"index" json:"category"`               // semantic type (see NormalizeGlossaryCategory)
	Definition        string       `gorm:"type:text" json:"definition"`         // LLM-generated, current best
	CuratedDefinition string       `gorm:"type:text" json:"curated_definition"` // manual override text
	ManualOverride    bool         `gorm:"default:false;index" json:"manual_override"`
	TagId             string       `gorm:"index" json:"tag_id,omitempty"` // set when Kind==entity; == Tag.Id (slug)
	Source            string       `json:"source"`                        // provenance
	DefinitionModel   string       `json:"definition_model,omitempty"`    // provider/model that produced Definition
	CreatedAt         time.Time    `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// TableName specifies the table name for GlossaryEntry
func (GlossaryEntry) TableName() string {
	return "glossary_entries"
}

// BeforeCreate ensures Id is set before creating a record
func (e *GlossaryEntry) BeforeCreate(tx *gorm.DB) error {
	if e.Id == "" {
		e.Id = uuid.New().String()
	}
	return nil
}

// EffectiveDefinition returns the curated override when present, else the generated one.
func (e *GlossaryEntry) EffectiveDefinition() string {
	if e.ManualOverride && e.CuratedDefinition != "" {
		return e.CuratedDefinition
	}
	return e.Definition
}

// glossaryKeySep collapses runs of whitespace/hyphens to a single space so that
// "Cobalt Strike", "cobalt-strike" and "cobalt   strike" all map to one key. This
// equivalence MUST agree with compileTagRegexp (notification/html.go) and the JS
// glossaryKey() normalizer in the digest template, otherwise a highlighted term in
// the prose will not resolve to its definition.
var glossaryKeySep = regexp.MustCompile(`[\s-]+`)

// NormalizeGlossaryKey produces the dedup/lookup identity for a term.
func NormalizeGlossaryKey(term string) string {
	s := strings.ToLower(strings.TrimSpace(term))
	s = strings.TrimPrefix(s, "#")
	s = glossaryKeySep.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
