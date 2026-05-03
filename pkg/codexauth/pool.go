package codexauth

import (
	"context"
	"downlink/pkg/models"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// PersistFn is called by the pool whenever credentials need to be persisted.
type PersistFn func(creds []models.CodexCredential) error

// Lease is a handle to a live credential for a single LLM call. Call one of
// the Mark* methods when done to update the credential's health state.
type Lease struct {
	CredID      string
	AccessToken string
	Headers     http.Header

	pool *Pool
}

func (l *Lease) MarkOK() {
	l.pool.mu.Lock()
	defer l.pool.mu.Unlock()
	for i := range l.pool.creds {
		if l.pool.creds[i].Id == l.CredID {
			c := &l.pool.creds[i]
			c.LastStatus = StatusOK
			now := time.Now()
			c.LastStatusAt = &now
			c.LastErrorReason = ""
			c.LastErrorResetAt = nil
			break
		}
	}
	_ = l.pool.persist(l.pool.creds)
}

func (l *Lease) MarkAuthFailed(reason string) {
	l.pool.mu.Lock()
	defer l.pool.mu.Unlock()
	for i := range l.pool.creds {
		if l.pool.creds[i].Id == l.CredID {
			c := &l.pool.creds[i]
			c.LastStatus = StatusAuthFailed
			now := time.Now()
			c.LastStatusAt = &now
			c.LastErrorReason = reason
			break
		}
	}
	_ = l.pool.persist(l.pool.creds)
}

func (l *Lease) MarkRateLimited(resetAt time.Time) {
	l.pool.mu.Lock()
	defer l.pool.mu.Unlock()
	for i := range l.pool.creds {
		if l.pool.creds[i].Id == l.CredID {
			c := &l.pool.creds[i]
			c.LastStatus = StatusRateLimited
			now := time.Now()
			c.LastStatusAt = &now
			c.LastErrorResetAt = &resetAt
			break
		}
	}
	_ = l.pool.persist(l.pool.creds)
}

// Pool manages a set of CodexCredentials for one provider config entry.
type Pool struct {
	mu      sync.Mutex
	creds   []models.CodexCredential
	persist PersistFn
}

// NewPool creates a pool backed by the given credentials and persist function.
func NewPool(creds []models.CodexCredential, persist PersistFn) *Pool {
	sorted := make([]models.CodexCredential, len(creds))
	copy(sorted, creds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return &Pool{creds: sorted, persist: persist}
}

// UpdateCredentials replaces the pool's credential set (e.g. after config reload).
func (p *Pool) UpdateCredentials(creds []models.CodexCredential) {
	sorted := make([]models.CodexCredential, len(creds))
	copy(sorted, creds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	p.mu.Lock()
	p.creds = sorted
	p.mu.Unlock()
}

// Acquire picks the best available credential, refreshing its access token if
// needed, and returns a Lease. Returns ErrNoCredentials when all credentials
// are either rate-limited or auth-failed.
func (p *Pool) Acquire(ctx context.Context) (*Lease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.creds {
		c := &p.creds[i]

		if c.LastStatus == StatusAuthFailed {
			continue
		}
		if c.LastStatus == StatusRateLimited {
			if c.LastErrorResetAt != nil && time.Now().Before(*c.LastErrorResetAt) {
				continue
			}
			// Reset-at has passed — clear rate-limit and try again.
			c.LastStatus = StatusOK
		}

		// Refresh if expiring soon.
		if ExpiresWithin(c.AccessToken, refreshSkew) {
			pair, err := RefreshTokens(ctx, c.RefreshToken)
			if err != nil {
				c.LastStatus = StatusAuthFailed
				now := time.Now()
				c.LastStatusAt = &now
				c.LastErrorReason = err.Error()
				_ = p.persist(p.creds)
				continue
			}
			c.AccessToken = pair.AccessToken
			c.RefreshToken = pair.RefreshToken
			c.LastRefresh = time.Now()
			_ = p.persist(p.creds)
		}

		return &Lease{
			CredID:      c.Id,
			AccessToken: c.AccessToken,
			Headers:     CodexHeaders(c.AccessToken),
			pool:        p,
		}, nil
	}

	return nil, fmt.Errorf("%w: all %d credentials are unhealthy", ErrNoCredentials, len(p.creds))
}

// AddCredential appends a new credential and persists.
func (p *Pool) AddCredential(cred models.CodexCredential) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creds = append(p.creds, cred)
	sort.Slice(p.creds, func(i, j int) bool {
		return p.creds[i].Priority < p.creds[j].Priority
	})
	return p.persist(p.creds)
}

// RemoveCredential removes a credential by ID and persists.
func (p *Pool) RemoveCredential(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, c := range p.creds {
		if c.Id == id {
			p.creds = append(p.creds[:i], p.creds[i+1:]...)
			return p.persist(p.creds)
		}
	}
	return fmt.Errorf("credential %s not found", id)
}

// SetPriority updates the priority of a credential by ID and persists.
func (p *Pool) SetPriority(id string, priority int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	found := false
	for i := range p.creds {
		if p.creds[i].Id == id {
			p.creds[i].Priority = priority
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("credential %s not found", id)
	}
	sort.Slice(p.creds, func(i, j int) bool {
		return p.creds[i].Priority < p.creds[j].Priority
	})
	return p.persist(p.creds)
}

// Credentials returns a snapshot of the current credential list.
func (p *Pool) Credentials() []models.CodexCredential {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]models.CodexCredential, len(p.creds))
	copy(out, p.creds)
	return out
}
