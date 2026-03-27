package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/session"
)

// ─── stubs ────────────────────────────────────────────────────────────────────

// stubBrowserInstance satisfies domain.BrowserInstance.
type stubBrowserInstance struct{ id string }

func (b *stubBrowserInstance) Context() context.Context { return context.Background() }
func (b *stubBrowserInstance) ID() string               { return b.id }
func (b *stubBrowserInstance) CreatedAt() time.Time     { return time.Time{} }
func (b *stubBrowserInstance) IsAlive() bool            { return true }
func (b *stubBrowserInstance) Close() error             { return nil }
func (b *stubBrowserInstance) Downloads() domain.DownloadManager { return nil }
func (b *stubBrowserInstance) Network() domain.NetworkManager   { return nil }

// stubPool is a minimal in-memory BrowserPool with release tracking.
type stubPool struct {
	mu       sync.Mutex
	released []string
	size     int
	counter  int
}

func newStubPool(size int) *stubPool {
	return &stubPool{size: size}
}

func (p *stubPool) Acquire(_ context.Context, _ ...string) (domain.BrowserInstance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counter++
	return &stubBrowserInstance{id: "browser-" + itoa(p.counter)}, nil
}

func (p *stubPool) Release(inst domain.BrowserInstance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.released = append(p.released, inst.ID())
}

func (p *stubPool) Size() int      { return p.size }
func (p *stubPool) Available() int { return p.size }
func (p *stubPool) Close() error   { return nil }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// stubPlanner returns a pre-built plan.
type stubPlanner struct{ plan *domain.Plan }

func (p *stubPlanner) Plan(_ context.Context, goal string, _ *domain.PageState) (*domain.Plan, error) {
	if p.plan != nil {
		return p.plan, nil
	}
	return &domain.Plan{Goal: goal, Steps: []domain.Step{}}, nil
}

// stubSequencer returns a pre-built RunResult.
type stubSequencer struct{ result *domain.RunResult }

func (s *stubSequencer) Run(_ context.Context, _ domain.BrowserInstance, plan *domain.Plan) (*domain.RunResult, error) {
	if s.result != nil {
		return s.result, nil
	}
	return &domain.RunResult{Plan: plan, Success: true, FailedStep: -1}, nil
}

// buildManager creates a DefaultSessionManager with stub dependencies.
func buildManager(maxConcurrent int) (*session.DefaultSessionManager, *stubPool) {
	pool := newStubPool(10)
	mgr := session.NewDefaultSessionManager(session.Config{
		Pool:          pool,
		Planner:       &stubPlanner{},
		Sequencer:     &stubSequencer{},
		MaxConcurrent: maxConcurrent,
	})
	return mgr, pool
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestCreate_IDAndStatus verifies Create returns a session with UUID and status="active".
func TestCreate_IDAndStatus(t *testing.T) {
	mgr, _ := buildManager(5)
	ctx := context.Background()

	s, err := mgr.Create(ctx, "test goal")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if s.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if s.Status != "active" {
		t.Errorf("expected status=active, got %s", s.Status)
	}
	if s.Goal != "test goal" {
		t.Errorf("expected goal=%q, got %q", "test goal", s.Goal)
	}
	if s.BrowserID == "" {
		t.Error("expected non-empty BrowserID")
	}
}

// TestGet_ReturnsSession verifies Get retrieves the correct session.
func TestGet_ReturnsSession(t *testing.T) {
	mgr, _ := buildManager(5)
	ctx := context.Background()

	s, _ := mgr.Create(ctx, "goal")
	got, err := mgr.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("expected ID %s, got %s", s.ID, got.ID)
	}
}

// TestGet_NotFound verifies Get returns ErrSessionNotFound for unknown IDs.
func TestGet_NotFound(t *testing.T) {
	mgr, _ := buildManager(5)
	_, err := mgr.Get(context.Background(), "nonexistent")
	if err != domain.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

// TestList_ReturnsAllSessions verifies List returns all created sessions.
func TestList_ReturnsAllSessions(t *testing.T) {
	mgr, _ := buildManager(5)
	ctx := context.Background()

	_, _ = mgr.Create(ctx, "goal1")
	_, _ = mgr.Create(ctx, "goal2")

	sessions, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

// TestExecute_EndToEnd verifies Execute runs plan→sequence and marks "completed".
func TestExecute_EndToEnd(t *testing.T) {
	pool := newStubPool(10)
	plan := &domain.Plan{Goal: "buy milk", Steps: []domain.Step{
		{Action: "navigate", Params: map[string]interface{}{"url": "https://example.com"}},
	}}
	seqResult := &domain.RunResult{Plan: plan, Success: true, FailedStep: -1}

	mgr := session.NewDefaultSessionManager(session.Config{
		Pool:          pool,
		Planner:       &stubPlanner{plan: plan},
		Sequencer:     &stubSequencer{result: seqResult},
		MaxConcurrent: 5,
	})

	ctx := context.Background()
	s, err := mgr.Create(ctx, "buy milk")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	result, err := mgr.Execute(ctx, s.ID)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}

	got, _ := mgr.Get(ctx, s.ID)
	if got.Status != "completed" {
		t.Errorf("expected status=completed, got %s", got.Status)
	}
	if got.Plan == nil {
		t.Error("expected Plan to be set after Execute")
	}
}

// TestConcurrentLimit verifies Create returns ErrConcurrentLimitExceeded at limit.
func TestConcurrentLimit(t *testing.T) {
	mgr, _ := buildManager(2)
	ctx := context.Background()

	_, err := mgr.Create(ctx, "goal1")
	if err != nil {
		t.Fatalf("Create 1 error: %v", err)
	}
	_, err = mgr.Create(ctx, "goal2")
	if err != nil {
		t.Fatalf("Create 2 error: %v", err)
	}
	_, err = mgr.Create(ctx, "goal3")
	if err != domain.ErrConcurrentLimitExceeded {
		t.Errorf("expected ErrConcurrentLimitExceeded, got %v", err)
	}
}

// TestDelete_ReleasesPool verifies Delete removes the session and releases browser.
func TestDelete_ReleasesPool(t *testing.T) {
	mgr, pool := buildManager(5)
	ctx := context.Background()

	s, _ := mgr.Create(ctx, "goal")
	if err := mgr.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	pool.mu.Lock()
	released := len(pool.released)
	pool.mu.Unlock()

	if released != 1 {
		t.Errorf("expected 1 released browser, got %d", released)
	}

	_, err := mgr.Get(ctx, s.ID)
	if err != domain.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
	}
}

// TestDelete_NotFound verifies Delete returns ErrSessionNotFound for unknown IDs.
func TestDelete_NotFound(t *testing.T) {
	mgr, _ := buildManager(5)
	err := mgr.Delete(context.Background(), "ghost")
	if err != domain.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

// TestConcurrentCreate is a data-race guard: multiple goroutines create sessions.
func TestConcurrentCreate(t *testing.T) {
	mgr, _ := buildManager(50)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mgr.Create(ctx, "concurrent goal")
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Create error: %v", err)
	}

	sessions, _ := mgr.List(ctx)
	if len(sessions) != 10 {
		t.Errorf("expected 10 sessions, got %d", len(sessions))
	}
}
