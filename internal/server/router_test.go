package server_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mwigge/llmctl/internal/config"
	"github.com/mwigge/llmctl/internal/server"
)

// makeSimpleRouter builds a router that sends all chat completions to upstream.
func makeSimpleRouter(upstream *httptest.Server) *server.RouterForTest {
	cfg := &config.Config{
		Server: config.ServerCfg{Host: "127.0.0.1", Port: 9910},
		Models: []config.ModelRef{
			{Alias: "m0", Path: "/m0.gguf", Role: "reason"},
		},
	}
	return server.NewRouterForTest(cfg, upstream.URL, upstream.URL)
}

// upstreamRecord records the last request received and replies with a fixed body.
type upstreamRecord struct {
	lastBody string
	reply    string
}

func (u *upstreamRecord) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.lastBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(u.reply))
	}
}

// makeRouterWithUpstreams builds a Router that rewrites upstream ports to the
// two fake httptest servers provided.
func makeRouterWithUpstreams(t *testing.T, code, reason *httptest.Server) *server.RouterForTest {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerCfg{
			Host: "127.0.0.1",
			Port: 9900,
		},
		Models: []config.ModelRef{
			{Alias: "reason-7b", Path: "/models/r.gguf", Role: "reason"},
			{Alias: "code-13b", Path: "/models/c.gguf", Role: "code"},
		},
	}
	return server.NewRouterForTest(cfg, reason.URL, code.URL)
}

func TestRouter_CodeKeyword_RoutesToCodeModel(t *testing.T) {
	t.Parallel()

	codeHit := &upstreamRecord{reply: `{"choices":[]}`}
	reasonHit := &upstreamRecord{reply: `{"choices":[]}`}

	codeServer := httptest.NewServer(codeHit.handler())
	defer codeServer.Close()
	reasonServer := httptest.NewServer(reasonHit.handler())
	defer reasonServer.Close()

	rt := makeRouterWithUpstreams(t, codeServer, reasonServer)

	body := `{"messages":[{"role":"user","content":"write a function in go"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	rt.ServeHTTP(rr, req)

	if codeHit.lastBody == "" {
		t.Error("expected request to reach code upstream, but it did not")
	}
	if reasonHit.lastBody != "" {
		t.Error("request unexpectedly reached reason upstream")
	}
}

func TestRouter_ReasonKeyword_RoutesToReasonModel(t *testing.T) {
	t.Parallel()

	codeHit := &upstreamRecord{reply: `{"choices":[]}`}
	reasonHit := &upstreamRecord{reply: `{"choices":[]}`}

	codeServer := httptest.NewServer(codeHit.handler())
	defer codeServer.Close()
	reasonServer := httptest.NewServer(reasonHit.handler())
	defer reasonServer.Close()

	rt := makeRouterWithUpstreams(t, codeServer, reasonServer)

	body := `{"messages":[{"role":"user","content":"explain why this algorithm works"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	rt.ServeHTTP(rr, req)

	if reasonHit.lastBody == "" {
		t.Error("expected request to reach reason upstream, but it did not")
	}
	if codeHit.lastBody != "" {
		t.Error("request unexpectedly reached code upstream")
	}
}

func TestRouter_AmbiguousInput_RoutesToFirstModel(t *testing.T) {
	t.Parallel()

	codeHit := &upstreamRecord{reply: `{"choices":[]}`}
	reasonHit := &upstreamRecord{reply: `{"choices":[]}`}

	codeServer := httptest.NewServer(codeHit.handler())
	defer codeServer.Close()
	reasonServer := httptest.NewServer(reasonHit.handler())
	defer reasonServer.Close()

	rt := makeRouterWithUpstreams(t, codeServer, reasonServer)

	// Ambiguous: no code keywords, no "reason" keywords → fallback to model[0] (reason role)
	body := `{"messages":[{"role":"user","content":"hello, what is the weather today?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	rt.ServeHTTP(rr, req)

	if reasonHit.lastBody == "" {
		t.Error("ambiguous input should have fallen back to reason (first) model, but reason was not hit")
	}
	if codeHit.lastBody != "" {
		t.Error("ambiguous input should not route to code upstream")
	}
}

func TestRouter_SSEHeaders_Preserved(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Custom-Header", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: hello\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerCfg{Host: "127.0.0.1", Port: 9901},
		Models: []config.ModelRef{
			{Alias: "m0", Path: "/m0.gguf", Role: "reason"},
		},
	}
	rt := server.NewRouterForTest(cfg, upstream.URL, upstream.URL)

	body := `{"messages":[{"role":"user","content":"tell me a story"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if rr.Header().Get("X-Custom-Header") != "yes" {
		t.Errorf("X-Custom-Header not preserved")
	}
}

func TestRouter_InjectsToolChoice_WhenToolsPresentAndChoiceAbsent(t *testing.T) {
	t.Parallel()

	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	rt := makeSimpleRouter(upstream)

	body := `{"messages":[{"role":"user","content":"call a tool"}],"tools":[{"type":"function","function":{"name":"get_weather"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	if !strings.Contains(receivedBody, `"tool_choice":"auto"`) {
		t.Errorf("expected tool_choice:auto injected; upstream received:\n%s", receivedBody)
	}
}

func TestRouter_DoesNotOverwriteExistingToolChoice(t *testing.T) {
	t.Parallel()

	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	rt := makeSimpleRouter(upstream)

	body := `{"messages":[{"role":"user","content":"call a tool"}],"tools":[{"type":"function","function":{"name":"get_weather"}}],"tool_choice":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	if strings.Contains(receivedBody, `"tool_choice":"auto"`) {
		t.Errorf("tool_choice should not be overwritten; upstream received:\n%s", receivedBody)
	}
	if !strings.Contains(receivedBody, `"tool_choice":"none"`) {
		t.Errorf("original tool_choice should be preserved; upstream received:\n%s", receivedBody)
	}
}

func TestRouter_NoInjection_WhenNoTools(t *testing.T) {
	t.Parallel()

	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	rt := makeSimpleRouter(upstream)

	body := `{"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)

	if strings.Contains(receivedBody, "tool_choice") {
		t.Errorf("tool_choice should not be added when no tools present; got:\n%s", receivedBody)
	}
}
