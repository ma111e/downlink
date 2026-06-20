package llmgateway

import "context"

// runIDKey is the private context key under which a digest run id is carried.
type runIDKey struct{}

// WithRunID returns a context tagged with runID. Every gateway call made under
// the returned context (and its descendants, including those created by
// context.WithTimeout / errgroup.WithContext) is correlated to that run by the
// recorder. An empty runID is a no-op tag.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromContext returns the run id tagged on ctx, or "" if none.
func RunIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKey{}).(string); ok {
		return v
	}
	return ""
}
