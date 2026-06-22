package scoring

import "testing"

func TestComputeRange(t *testing.T) {
	tests := []struct {
		name string
		dims Dimensions
		want int
	}{
		{
			name: "all max",
			dims: Dimensions{4, 4, 4, 4, 4, 4, false, false},
			want: 100,
		},
		{
			name: "all zero",
			dims: Dimensions{0, 0, 0, 0, 0, 0, false, false},
			want: 0,
		},
		{
			name: "all mid",
			dims: Dimensions{2, 2, 2, 2, 2, 2, false, false},
			want: 50,
		},
		{
			name: "breaking exploitation event scores must-read",
			dims: Dimensions{Specificity: 4, Severity: 4, Breadth: 4, Novelty: 3, Actionability: 4, Credibility: 3},
			want: 95,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compute(tt.dims); got != tt.want {
				t.Errorf("Compute() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeAggregatorOverride(t *testing.T) {
	// High dimensions but flagged as aggregator must collapse to AggregatorScore.
	d := Dimensions{4, 4, 4, 4, 4, 4, true, false}
	if got := Compute(d); got != AggregatorScore {
		t.Errorf("aggregator Compute() = %d, want %d", got, AggregatorScore)
	}
}

func TestComputeEvergreenCap(t *testing.T) {
	// Specificity 0 (pure evergreen) but otherwise excellent: would score 80,
	// must be capped at EvergreenCap.
	d := Dimensions{Specificity: 0, Severity: 4, Breadth: 4, Novelty: 4, Actionability: 4, Credibility: 4}
	if got := Compute(d); got != EvergreenCap {
		t.Errorf("evergreen Compute() = %d, want %d", got, EvergreenCap)
	}
}

func TestComputePromoCap(t *testing.T) {
	// A promotional article that otherwise scores in the Must Read range must be
	// capped at PromoCap (the top of the May Read tier).
	d := Dimensions{Specificity: 4, Severity: 4, Breadth: 4, Novelty: 4, Actionability: 4, Credibility: 4, IsPromotional: true}
	if got := Compute(d); got != PromoCap {
		t.Errorf("promotional Compute() = %d, want %d", got, PromoCap)
	}
	if ReadTier(PromoCap) != "May Read" {
		t.Errorf("PromoCap %d is not in the May Read tier", PromoCap)
	}
	// A promotional article that already scores below the cap is left untouched.
	low := Dimensions{Specificity: 1, Severity: 0, Breadth: 1, Novelty: 0, Actionability: 0, Credibility: 1, IsPromotional: true}
	if got := Compute(low); got != Compute(Dimensions{Specificity: 1, Severity: 0, Breadth: 1, Novelty: 0, Actionability: 0, Credibility: 1}) {
		t.Errorf("below-cap promotional article should be unchanged, got %d", got)
	}
}

func TestComputeClampsOutOfRange(t *testing.T) {
	// Out-of-range values clamp to [0,4]; this should equal the all-max score.
	d := Dimensions{Specificity: 99, Severity: 99, Breadth: 99, Novelty: 99, Actionability: 99, Credibility: 99}
	if got := Compute(d); got != 100 {
		t.Errorf("clamped Compute() = %d, want 100", got)
	}
	neg := Dimensions{Specificity: -5, Severity: -5, Breadth: -5, Novelty: -5, Actionability: -5, Credibility: -5}
	if got := Compute(neg); got != 0 {
		t.Errorf("negative-clamped Compute() = %d, want 0", got)
	}
}

func TestReadTierBoundaries(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{100, "Must Read"},
		{90, "Must Read"},
		{89, "Should Read"},
		{75, "Should Read"},
		{74, "May Read"},
		{60, "May Read"},
		{59, "Optional"},
		{1, "Optional"},
		{0, "Unscored"},
	}
	for _, tt := range tests {
		if got := ReadTier(tt.score); got != tt.want {
			t.Errorf("ReadTier(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestPriorityKeyBoundaries(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{90, "must"},
		{89, "should"},
		{75, "should"},
		{74, "may"},
		{60, "may"},
		{59, "opt"},
		{0, "opt"},
	}
	for _, tt := range tests {
		if got := PriorityKey(tt.score); got != tt.want {
			t.Errorf("PriorityKey(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestDefaultConfigParity(t *testing.T) {
	// The Config methods on DefaultConfig must match the package-level wrappers
	// across the whole score range and every dimension permutation edge.
	cfg := DefaultConfig()
	dimSets := []Dimensions{
		{4, 4, 4, 4, 4, 4, false, false},
		{0, 0, 0, 0, 0, 0, false, false},
		{2, 2, 2, 2, 2, 2, false, false},
		{4, 4, 4, 3, 4, 3, false, false},
		{4, 4, 4, 4, 4, 4, true, false},  // aggregator override
		{0, 4, 4, 4, 4, 4, false, false}, // evergreen cap
		{4, 4, 4, 4, 4, 4, false, true},  // promotional cap
		{1, 3, 0, 2, 4, 1, false, false},
	}
	for _, d := range dimSets {
		if cfg.Compute(d) != Compute(d) {
			t.Errorf("Config.Compute(%+v) = %d, package Compute = %d", d, cfg.Compute(d), Compute(d))
		}
	}
	for score := 0; score <= 100; score++ {
		if cfg.ReadTier(score) != ReadTier(score) {
			t.Errorf("Config.ReadTier(%d) = %q, package ReadTier = %q", score, cfg.ReadTier(score), ReadTier(score))
		}
		if cfg.PriorityKey(score) != PriorityKey(score) {
			t.Errorf("Config.PriorityKey(%d) = %q, package PriorityKey = %q", score, cfg.PriorityKey(score), PriorityKey(score))
		}
	}
}

func TestConfigCustomWeights(t *testing.T) {
	// A profile that cares only about severity: all weight on Severity. A mid
	// article (all 2s) then scores purely from severity = 2/4 = 50.
	cfg := DefaultConfig()
	cfg.Weights = DimensionWeights{Severity: 1.0}
	if got := cfg.Compute(Dimensions{2, 2, 2, 2, 2, 2, false, false}); got != 50 {
		t.Errorf("severity-only Compute = %d, want 50", got)
	}
	// Specificity 4 avoids the evergreen cap; severity 4 with all weight on
	// severity yields a full 100.
	if got := cfg.Compute(Dimensions{4, 4, 0, 0, 0, 0, false, false}); got != 100 {
		t.Errorf("severity-only max-severity Compute = %d, want 100", got)
	}
}

func TestConfigCustomThresholds(t *testing.T) {
	// A lenient profile that promotes more articles to higher tiers.
	cfg := DefaultConfig()
	cfg.TierMust, cfg.TierShould, cfg.TierMay = 70, 50, 30
	cases := []struct {
		score int
		tier  string
		key   string
	}{
		{70, "Must Read", "must"},
		{69, "Should Read", "should"},
		{50, "Should Read", "should"},
		{49, "May Read", "may"},
		{30, "May Read", "may"},
		{29, "Optional", "opt"},
	}
	for _, c := range cases {
		if got := cfg.ReadTier(c.score); got != c.tier {
			t.Errorf("ReadTier(%d) = %q, want %q", c.score, got, c.tier)
		}
		if got := cfg.PriorityKey(c.score); got != c.key {
			t.Errorf("PriorityKey(%d) = %q, want %q", c.score, got, c.key)
		}
	}
}

func TestWeightsSumToOne(t *testing.T) {
	sum := Weights.Specificity + Weights.Severity + Weights.Breadth +
		Weights.Novelty + Weights.Actionability + Weights.Credibility
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("weights sum = %f, want 1.0", sum)
	}
}
