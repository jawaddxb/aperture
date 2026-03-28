package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/domain"
)

// ─── stub SessionManager ──────────────────────────────────────────────────────

type stubSession struct {
	id     string
	status string
	goal   string
}

// stubSessionManager provides a controllable domain.SessionManager.
type stubSessionManager struct {
	sessions map[string]*domain.Session
	execErr  error
	execFn   func(id string) (*domain.RunResult, error)
}

func newStubManager() *stubSessionManager {
	return &stubSessionManager{sessions: make(map[string]*domain.Session)}
}

func (m *stubSessionManager) Create(_ context.Context, goal string, _ map[string]string) (*domain.Session, error) {
	if m.sessions == nil {
		m.sessions = make(map[string]*domain.Session)
	}
	s := &domain.Session{
		ID:        "test-uuid-1234",
		Status:    "active",
		Goal:      goal,
		BrowserID: "browser-1",
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.sessions[s.ID] = s
	return s, nil
}

func (m *stubSessionManager) Get(_ context.Context, id string) (*domain.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return s, nil
}

func (m *stubSessionManager) List(_ context.Context) ([]*domain.Session, error) {
	out := make([]*domain.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out, nil
}

func (m *stubSessionManager) Update(_ context.Context, s *domain.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *stubSessionManager) Delete(_ context.Context, id string) error {
	if _, ok := m.sessions[id]; !ok {
		return domain.ErrSessionNotFound
	}
	delete(m.sessions, id)
	return nil
}

func (m *stubSessionManager) Execute(ctx context.Context, id string) (*domain.RunResult, error) {
	if m.execFn != nil {
		return m.execFn(id)
	}
	if m.execErr != nil {
		return nil, m.execErr
	}
	plan := &domain.Plan{Goal: "stub-goal", Steps: []domain.Step{}}
	return &domain.RunResult{Plan: plan, Success: true, FailedStep: -1, Duration: time.Millisecond * 10}, nil
}

// ─── helper ───────────────────────────────────────────────────────────────────

func buildRouter(mgr domain.SessionManager) http.Handler {
	return api.NewRouter(api.RouterConfig{SessionManager: mgr})
}

func postJSON(router http.Handler, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getReq(router http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func deleteReq(router http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestCreateSession_Returns201 verifies POST /api/v1/sessions returns 201 with session_id.
func TestCreateSession_Returns201(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := postJSON(router, "/api/v1/sessions", map[string]interface{}{
		"goal": "fill login form",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["session_id"] == "" {
		t.Error("expected non-empty session_id")
	}
	if resp["status"] != "active" {
		t.Errorf("expected status=active, got %v", resp["status"])
	}
}

// TestCreateSession_MissingGoalReturns400 verifies empty goal returns 400.
func TestCreateSession_MissingGoalReturns400(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := postJSON(router, "/api/v1/sessions", map[string]interface{}{})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "goal") {
		t.Errorf("expected error to mention 'goal', got: %s", body)
	}
}

// TestCreateSession_InvalidBodyReturns400 verifies malformed JSON returns 400.
func TestCreateSession_InvalidBodyReturns400(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestGetSession_ReturnsDetails verifies GET /api/v1/sessions/:id returns session details.
func TestGetSession_ReturnsDetails(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	// Create a session first.
	createRec := postJSON(router, "/api/v1/sessions", map[string]interface{}{"goal": "test goal"})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", createRec.Code)
	}

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	id := createResp["session_id"].(string)

	rec := getReq(router, "/api/v1/sessions/"+id)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["session_id"] != id {
		t.Errorf("expected session_id=%s, got %v", id, resp["session_id"])
	}
}

// TestGetSession_NotFoundReturns404 verifies unknown ID returns 404.
func TestGetSession_NotFoundReturns404(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := getReq(router, "/api/v1/sessions/nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestDeleteSession_Returns204 verifies DELETE /api/v1/sessions/:id returns 204.
func TestDeleteSession_Returns204(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	createRec := postJSON(router, "/api/v1/sessions", map[string]interface{}{"goal": "goal"})
	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	id := createResp["session_id"].(string)

	rec := deleteReq(router, "/api/v1/sessions/"+id)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d — body: %s", rec.Code, rec.Body.String())
	}

	// Confirm gone.
	getRecAfter := getReq(router, "/api/v1/sessions/"+id)
	if getRecAfter.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getRecAfter.Code)
	}
}

// TestDeleteSession_NotFoundReturns404 verifies deleting unknown session returns 404.
func TestDeleteSession_NotFoundReturns404(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	rec := deleteReq(router, "/api/v1/sessions/ghost")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestListSessions_ReturnsAll verifies GET /api/v1/sessions lists all sessions.
func TestListSessions_ReturnsAll(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	postJSON(router, "/api/v1/sessions", map[string]interface{}{"goal": "goal1"})
	postJSON(router, "/api/v1/sessions", map[string]interface{}{"goal": "goal2"})

	sessions, _ := mgr.List(context.Background())
	if len(sessions) < 1 {
		t.Fatalf("expected at least 1 session in manager, got %d", len(sessions))
	}

	rec := getReq(router, "/api/v1/sessions")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp []interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp) != len(sessions) {
		t.Errorf("expected %d sessions in response, got %d", len(sessions), len(resp))
	}
}

// TestExecuteSession_ReturnsRunResult verifies POST /api/v1/sessions/:id/execute returns result.
func TestExecuteSession_ReturnsRunResult(t *testing.T) {
	mgr := newStubManager()
	router := buildRouter(mgr)

	createRec := postJSON(router, "/api/v1/sessions", map[string]interface{}{"goal": "run me"})
	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	id := createResp["session_id"].(string)

	rec := postJSON(router, "/api/v1/sessions/"+id+"/execute", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}
