package searchtest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

type FakeEngine struct {
	URL string

	status int
	body   string
	mu     sync.Mutex
	asked  []url.Values
}

func NewFakeEngine(t *testing.T, status int, body string) *FakeEngine {
	t.Helper()

	engine := &FakeEngine{status: status, body: body}
	server := httptest.NewServer(http.HandlerFunc(engine.answer))
	t.Cleanup(server.Close)
	engine.URL = server.URL

	return engine
}

func (e *FakeEngine) Asked() []url.Values {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]url.Values(nil), e.asked...)
}

func (e *FakeEngine) answer(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	e.asked = append(e.asked, r.URL.Query())
	e.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.status)
	_, _ = w.Write([]byte(e.body))
}
