package claudeauth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func future(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}

func TestPool_AcquireBasic(t *testing.T) {
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.CredID != "a1" {
		t.Fatalf("expected cred a1, got %s", lease.CredID)
	}
	// Verify identity headers are attached.
	if lease.Headers.Get("anthropic-beta") == "" {
		t.Fatal("expected anthropic-beta header on lease")
	}
	if lease.Headers.Get("x-app") != "cli" {
		t.Fatalf("expected x-app: cli, got %q", lease.Headers.Get("x-app"))
	}
}

func TestPool_SkipsRateLimited(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, LastStatus: StatusRateLimited, LastErrorResetAt: &resetAt,
			AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
		{Id: "a2", Priority: 1, AccessToken: "at2", RefreshToken: "rt2", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.CredID != "a2" {
		t.Fatalf("expected fallback a2, got %s", lease.CredID)
	}
}

func TestPool_ClearsExpiredRateLimit(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, LastStatus: StatusRateLimited, LastErrorResetAt: &past,
			AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("expected rate-limit to be cleared, got error: %v", err)
	}
	if lease.CredID != "a1" {
		t.Fatalf("expected a1 after rate-limit cleared, got %s", lease.CredID)
	}
}

func TestPool_SkipsAuthFailed(t *testing.T) {
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, LastStatus: StatusAuthFailed,
			AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	_, err := pool.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected ErrNoCredentials for all-auth-failed pool")
	}
}

// TestPool_RefreshFailureMarksAuthFailed verifies that an expiring token whose
// refresh fails (network/OAuth error) marks the credential auth-failed rather
// than handing out a stale token. ExpiresAt in the past forces the refresh path,
// which hits the real token endpoint and fails closed.
func TestPool_RefreshFailureMarksAuthFailed(t *testing.T) {
	t.Setenv("DOWNLINK_CLAUDE_TOKEN_URL", "http://127.0.0.1:0/invalid")
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(-time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	_, err := pool.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected error when the only credential cannot be refreshed")
	}
	creds := pool.Credentials()
	if creds[0].LastStatus != StatusAuthFailed {
		t.Fatalf("expected credential marked auth_failed, got %q", creds[0].LastStatus)
	}
}

func TestPool_ConcurrentAcquireNoRace(t *testing.T) {
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := pool.Acquire(context.Background())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			lease.MarkOK()
		}()
	}
	wg.Wait()
}

func TestPool_SetPriority(t *testing.T) {
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
		{Id: "a2", Priority: 1, AccessToken: "at2", RefreshToken: "rt2", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	if err := pool.SetPriority("a2", -1); err != nil {
		t.Fatalf("SetPriority failed: %v", err)
	}
	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.CredID != "a2" {
		t.Fatalf("expected a2 (now priority -1) to be picked first, got %s", lease.CredID)
	}
}

func TestPool_RemoveCredential(t *testing.T) {
	pool := NewPool([]models.CodexCredential{
		{Id: "a1", Priority: 0, AccessToken: "at1", RefreshToken: "rt1", ExpiresAt: future(time.Hour)},
	}, func(c []models.CodexCredential) error { return nil })

	if err := pool.RemoveCredential("a1"); err != nil {
		t.Fatalf("RemoveCredential failed: %v", err)
	}
	_, err := pool.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected ErrNoCredentials after removal")
	}
}
