package bridge_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/bridge"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// ─── stub SessionManager ──────────────────────────────────────────────────────

type stubSession struct {
	mu        sync.RWMutex
	sessions  map[string]*domain.Session
	execDelay time.Duration
	execErr   error
}

func newStub() *stubSession {
	return &stubSession{sessions: make(map[string]*domain.Session)}
}

func (s *stubSession) Create(_ context.Context, goal string) (*domain.Session, error) {
	key := goal
	if len(key) > 8 {
		key = key[:8]
	}
	sess := &domain.Session{
		ID:        "sess-" + key,
		Status:    "active",
		Goal:      goal,
		BrowserID: "browser-1",
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *stubSession) Get(_ context.Context, id string) (*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}
	return nil, domain.ErrSessionNotFound
}

func (s *stubSession) List(_ context.Context) ([]*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Session, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, v)
	}
	return out, nil
}

func (s *stubSession) Update(_ context.Context, sess *domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return nil
}

func (s *stubSession) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *stubSession) Execute(ctx context.Context, _ string) (*domain.RunResult, error) {
	if s.execDelay > 0 {
		select {
		case <-time.After(s.execDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.execErr != nil {
		return nil, s.execErr
	}
	return &domain.RunResult{
		Success:    true,
		FailedStep: -1,
		Steps: []domain.StepResult{
			{
				Index:    0,
				Step:     domain.Step{Action: "navigate"},
				Duration: 10 * time.Millisecond,
				Result: &domain.ActionResult{
					Action:  "navigate",
					Success: true,
					PageState: &domain.PageState{
						URL:   "https://example.com",
						Title: "Example",
					},
				},
			},
		},
	}, nil
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestExecuteTask_ReturnsTaskResponse(t *testing.T) {
	t.Parallel()
	stub := newStub()
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: stub})

	req := &domain.TaskRequest{
		ID:   "task-001",
		Goal: "navigate to example.com",
	}

	resp, err := b.ExecuteTask(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.ID != "task-001" {
		t.Errorf("expected id=task-001 got %q", resp.ID)
	}
	if !resp.Success {
		t.Errorf("expected success=true, error=%q", resp.Error)
	}
	if len(resp.Steps) == 0 {
		t.Error("expected at least one step summary")
	}
	if resp.FinalURL != "https://example.com" {
		t.Errorf("expected final_url=https://example.com got %q", resp.FinalURL)
	}
}

func TestExecuteTask_GeneratesIDWhenEmpty(t *testing.T) {
	t.Parallel()
	stub := newStub()
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: stub})

	resp, err := b.ExecuteTask(context.Background(), &domain.TaskRequest{Goal: "click button"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected auto-generated task ID")
	}
}

func TestExecuteTask_SessionError_ReturnsErrorInResponse(t *testing.T) {
	t.Parallel()
	stub := newStub()
	stub.execErr = errors.New("browser pool exhausted")
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: stub})

	resp, err := b.ExecuteTask(context.Background(), &domain.TaskRequest{
		ID:   "task-err",
		Goal: "click something",
	})
	if err != nil {
		t.Fatalf("unexpected non-nil error: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false on executor error")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error field")
	}
}

func TestCancelTask_ViaContext(t *testing.T) {
	t.Parallel()
	stub := newStub()
	stub.execDelay = 5 * time.Second // long enough to cancel
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: stub})

	cancelCtx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan *domain.TaskResponse, 1)

	go func() {
		resp, _ := b.ExecuteTask(cancelCtx, &domain.TaskRequest{
			ID:   "cancel-ctx",
			Goal: "ctx cancelled task",
		})
		doneCh <- resp
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case resp := <-doneCh:
		if resp != nil && resp.Success {
			t.Error("expected cancelled task to not succeed")
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for cancelled task to finish")
	}
}

func TestGetStatus_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: newStub()})
	_, err := b.GetStatus(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for unknown task ID")
	}
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound got %v", err)
	}
}

func TestCancelTask_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	b := bridge.NewOpenClawBridge(bridge.Config{SessionManager: newStub()})
	err := b.CancelTask(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for unknown task ID")
	}
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound got %v", err)
	}
}

func TestConcurrentTaskLimit(t *testing.T) {
	t.Parallel()
	stub := newStub()
	stub.execDelay = 500 * time.Millisecond
	b := bridge.NewOpenClawBridge(bridge.Config{
		SessionManager: stub,
		MaxConcurrent:  2,
	})

	var wg sync.WaitGroup
	startedCh := make(chan struct{}, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			startedCh <- struct{}{}
			_, err := b.ExecuteTask(context.Background(), &domain.TaskRequest{
				ID:   fmt.Sprintf("t%d", i),
				Goal: fmt.Sprintf("navigate task %d", i),
			})
			if err != nil {
				t.Errorf("unexpected error from allowed task %d: %v", i, err)
			}
		}(i)
	}

	// Wait for both goroutines to start their tasks.
	<-startedCh
	<-startedCh
	time.Sleep(50 * time.Millisecond)

	// Third task should be rejected while the two are in-flight.
	_, err := b.ExecuteTask(context.Background(), &domain.TaskRequest{
		ID:   "overflow",
		Goal: "should be rejected",
	})
	if err == nil {
		t.Error("expected concurrent limit error for third task")
	}

	wg.Wait()
}
