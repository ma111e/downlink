// Package scoring owns the article importance model: the rubric dimensions an
// LLM rates, the deterministic aggregation of those dimensions into a 0-100
// score, and the tier thresholds used to bucket articles in digests.
//
// The LLM no longer chooses the final score directly. It rates a handful of
// narrow, anchored sub-dimensions (0-4 each) and Compute combines them with
// tunable weights here. Because the raw dimensions are persisted alongside the
// computed score, the weights can be retuned later and scores recomputed in a
// batch without re-running the LLM.
package scoring

import "math"

// Dimensions are the rubric sub-scores rated by the LLM. Each numeric field is
// on a 0-4 scale; values outside that range are clamped by Compute.
type Dimensions struct {
	// Specificity: generic/evergreen concept (0) → single concrete, recent event (4).
	Specificity int `json:"specificity"`
	// Severity: informational (0) → active exploitation / critical patch / major breach (4).
	Severity int `json:"severity"`
	// Breadth: niche product (0) → ubiquitous software or whole sector affected (4).
	Breadth int `json:"breadth"`
	// Novelty: rehash of known facts (0) → genuinely new disclosure/finding (4).
	Novelty int `json:"novelty"`
	// Actionability: nothing to do (0) → clear defensive action, patch, IOCs, detection (4).
	Actionability int `json:"actionability"`
	// Credibility: unsourced blogspam (0) → primary source / vendor advisory / named researcher (4).
	Credibility int `json:"credibility"`
	// IsAggregator marks roundups / weekly recaps / link digests, which are forced
	// to AggregatorScore regardless of the other dimensions.
	IsAggregator bool `json:"is_aggregator"`
}

// Per-dimension weights, summing to 1.0. This is the single place to retune the
// relative influence of each dimension on the final score.
var Weights = struct {
	Specificity   float64
	Severity      float64
	Breadth       float64
	Novelty       float64
	Actionability float64
	Credibility   float64
}{
	Specificity:   0.20,
	Severity:      0.25,
	Breadth:       0.20,
	Novelty:       0.10,
	Actionability: 0.15,
	Credibility:   0.10,
}

const (
	// dimMax is the top of each dimension's 0-4 scale.
	dimMax = 4

	// AggregatorScore is the fixed score forced for aggregator/roundup articles,
	// preserving the previous prompt's "always set the score to exactly 40" rule.
	AggregatorScore = 40

	// EvergreenCap caps the score of pure-evergreen articles (Specificity == 0),
	// preserving the previous prompt's "generic/evergreen must score ≤60" rule.
	EvergreenCap = 60
)

// Read-tier thresholds (inclusive lower bounds) on the 0-100 score.
const (
	TierMustRead   = 90
	TierShouldRead = 75
	TierMayRead    = 60
)

func clampDim(v int) float64 {
	if v < 0 {
		return 0
	}
	if v > dimMax {
		return dimMax
	}
	return float64(v)
}

// DimensionWeights holds the per-dimension weights used by Config.Compute. It is
// the named-type form of the package-level Weights var, so a profile can carry
// its own weighting without touching package state.
type DimensionWeights struct {
	Specificity   float64
	Severity      float64
	Breadth       float64
	Novelty       float64
	Actionability float64
	Credibility   float64
}

// Config is a self-contained, retunable instance of the importance model: the
// dimension weights, the aggregator/evergreen overrides, and the read-tier
// thresholds. Profiles each resolve their own Config; callers that want the
// historical global behaviour use DefaultConfig (and the package-level Compute /
// ReadTier / PriorityKey wrappers, which delegate to it).
type Config struct {
	Weights         DimensionWeights
	AggregatorScore int
	EvergreenCap    int
	TierMust        int
	TierShould      int
	TierMay         int
}

// DefaultConfig returns the historical global model, sourced from the package
// Weights var and the Aggregator/Evergreen/Tier constants so there is a single
// source of truth for the defaults.
func DefaultConfig() Config {
	return Config{
		Weights: DimensionWeights{
			Specificity:   Weights.Specificity,
			Severity:      Weights.Severity,
			Breadth:       Weights.Breadth,
			Novelty:       Weights.Novelty,
			Actionability: Weights.Actionability,
			Credibility:   Weights.Credibility,
		},
		AggregatorScore: AggregatorScore,
		EvergreenCap:    EvergreenCap,
		TierMust:        TierMustRead,
		TierShould:      TierShouldRead,
		TierMay:         TierMayRead,
	}
}

// Compute aggregates rubric dimensions into a 0-100 importance score using the
// default global model. See Config.Compute.
func Compute(d Dimensions) int { return DefaultConfig().Compute(d) }

// ReadTier returns the human-facing priority label for a 0-100 score using the
// default global thresholds. See Config.ReadTier.
func ReadTier(score int) string { return DefaultConfig().ReadTier(score) }

// PriorityKey returns the short manifest bucket key for a 0-100 score using the
// default global thresholds. See Config.PriorityKey.
func PriorityKey(score int) string { return DefaultConfig().PriorityKey(score) }

// Compute aggregates rubric dimensions into a 0-100 importance score.
//
// Each dimension is normalised to 0-1, combined via c.Weights into a weighted
// average, and scaled to 0-100. Overrides are then applied: aggregator articles
// are forced to c.AggregatorScore, and pure-evergreen articles (Specificity == 0)
// are capped at c.EvergreenCap.
func (c Config) Compute(d Dimensions) int {
	if d.IsAggregator {
		return c.AggregatorScore
	}

	weighted := c.Weights.Specificity*clampDim(d.Specificity) +
		c.Weights.Severity*clampDim(d.Severity) +
		c.Weights.Breadth*clampDim(d.Breadth) +
		c.Weights.Novelty*clampDim(d.Novelty) +
		c.Weights.Actionability*clampDim(d.Actionability) +
		c.Weights.Credibility*clampDim(d.Credibility)

	score := int(math.Round(weighted / dimMax * 100))

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// Pure-evergreen content cannot exceed the "May Read" floor regardless of how
	// well it scores on other dimensions.
	if d.Specificity == 0 && score > c.EvergreenCap {
		score = c.EvergreenCap
	}

	return score
}

// ReadTier returns the human-facing priority label for a 0-100 score, used for
// digest table-of-contents grouping.
func (c Config) ReadTier(score int) string {
	switch {
	case score >= c.TierMust:
		return "Must Read"
	case score >= c.TierShould:
		return "Should Read"
	case score >= c.TierMay:
		return "May Read"
	case score > 0:
		return "Optional"
	default:
		return "Unscored"
	}
}

// PriorityKey returns the short bucket key for a 0-100 score, used for digest
// manifest priority tallies. Unlike ReadTier it has no "unscored" bucket;
// anything below TierMay falls into "opt".
func (c Config) PriorityKey(score int) string {
	switch {
	case score >= c.TierMust:
		return "must"
	case score >= c.TierShould:
		return "should"
	case score >= c.TierMay:
		return "may"
	default:
		return "opt"
	}
}
