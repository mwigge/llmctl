package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mwigge/llmctl/internal/config"
)

// RouterForTest is a testable variant of Router that accepts injected upstream URLs.
// The reason URL is used for non-code requests; the code URL for code requests.
type RouterForTest struct {
	handler http.Handler
}

// NewRouterForTest builds a RouterForTest with explicit upstream base URLs.
// reasonURL is model[0], codeURL is model[1].
func NewRouterForTest(cfg *config.Config, reasonURL, codeURL string) *RouterForTest {
	h := &routerHandler{cfg: cfg, reasonURL: reasonURL, codeURL: codeURL}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", h.handleChatCompletions)
	mux.HandleFunc("/", h.handlePassthrough)
	return &RouterForTest{handler: mux}
}

// ServeHTTP implements http.Handler so RouterForTest can be used with httptest.
func (r *RouterForTest) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

// routerHandler holds upstream URLs for proxying.
type routerHandler struct {
	cfg       *config.Config
	reasonURL string
	codeURL   string
}

func (h *routerHandler) handleChatCompletions(w http.ResponseWriter, req *http.Request) {
	target := h.routeTarget(req)
	req = injectToolChoice(req)
	proxyRequest(w, req, target)
}

// injectToolChoice reads the request body and, when the JSON payload contains a
// non-empty "tools" array and no "tool_choice" field, sets tool_choice to "auto".
// This is required for backends that ignore tools when tool_choice is absent.
// The function always returns a valid request; on any JSON parse error it returns
// the original request unchanged.
func injectToolChoice(req *http.Request) *http.Request {
	if req.Body == nil {
		return req
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 64*1024))
	if err != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		return req
	}
	// Restore body for downstream use (proxyRequest reads it again).
	req.Body = io.NopCloser(bytes.NewReader(body))

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return req
	}

	// Only inject when "tools" is present and non-empty.
	toolsRaw, hasTools := payload["tools"]
	if !hasTools {
		return req
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(toolsRaw, &tools); err != nil || len(tools) == 0 {
		return req
	}

	// Only inject when "tool_choice" is absent.
	if _, hasChoice := payload["tool_choice"]; hasChoice {
		return req
	}

	payload["tool_choice"] = json.RawMessage(`"auto"`)
	modified, err := json.Marshal(payload)
	if err != nil {
		return req
	}

	req.Body = io.NopCloser(bytes.NewReader(modified))
	req.ContentLength = int64(len(modified))
	return req
}

func (h *routerHandler) handlePassthrough(w http.ResponseWriter, req *http.Request) {
	proxyRequest(w, req, h.reasonURL)
}

func (h *routerHandler) routeTarget(req *http.Request) string {
	if req.Body == nil {
		return h.reasonURL
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 64*1024))
	if err != nil {
		return h.reasonURL
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	if looksLikeCode(string(body)) {
		return h.codeURL
	}
	return h.reasonURL
}

// codeKeywords are words in the last user message that indicate a coding task.
var codeKeywords = []string{
	"code", "implement", "write", "debug", "function",
	"refactor", "snippet", "program", "compile", "syntax",
}

// looksLikeCode returns true if the raw JSON body contains coding keywords.
func looksLikeCode(body string) bool {
	lower := strings.ToLower(body)
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// Router is a lightweight HTTP reverse proxy that routes /v1/chat/completions
// to either a "code" model or a "reason" model based on message content.
// It listens on cfg.Server.Port and forwards to model processes on Port+1, Port+2, ...
type Router struct {
	cfg    *config.Config
	server *http.Server
}

// NewRouter creates a Router that will listen on cfg.Server.Port.
func NewRouter(cfg *config.Config) *Router {
	return &Router{cfg: cfg}
}

// Serve starts the HTTP server and blocks until ctx is cancelled.
// It delegates all routing logic to routerHandler.
func (r *Router) Serve(ctx context.Context) error {
	h := &routerHandler{
		cfg:       r.cfg,
		reasonURL: r.reasonModelURL(),
		codeURL:   r.codeModelURL(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", h.handleChatCompletions)
	mux.HandleFunc("/", h.handlePassthrough)

	r.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", r.cfg.Server.Host, r.cfg.Server.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return r.server.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// upstreamURL returns the base URL for the n-th model process.
// +1 because the router itself occupies cfg.Server.Port.
func (r *Router) upstreamURL(n int) string {
	port := r.cfg.Server.Port + n + 1
	return fmt.Sprintf("http://%s:%d", r.cfg.Server.Host, port)
}

// codeModelURL returns the URL for the model with Role="code", or model[1].
func (r *Router) codeModelURL() string {
	for i, m := range r.cfg.Models {
		if strings.EqualFold(m.Role, "code") {
			return r.upstreamURL(i)
		}
	}
	if len(r.cfg.Models) > 1 {
		return r.upstreamURL(1)
	}
	return r.upstreamURL(0)
}

// reasonModelURL returns the URL for the model with Role="reason", or model[0].
func (r *Router) reasonModelURL() string {
	for i, m := range r.cfg.Models {
		if strings.EqualFold(m.Role, "reason") {
			return r.upstreamURL(i)
		}
	}
	return r.upstreamURL(0)
}

// proxyRequest forwards req to upstream+path, copies response headers and streams the body.
func proxyRequest(w http.ResponseWriter, req *http.Request, upstream string) {
	outURL := upstream + req.URL.RequestURI()

	outReq, err := http.NewRequestWithContext(req.Context(), req.Method, outURL, req.Body)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	copyHeaders(outReq.Header, req.Header)

	client := &http.Client{Timeout: 0} // no timeout: SSE streams can be long
	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// copyHeaders copies response headers from src into dst, skipping hop-by-hop headers.
func copyHeaders(dst, src http.Header) {
	hopByHop := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}
	for key, vals := range src {
		if hopByHop[key] {
			continue
		}
		for _, v := range vals {
			dst.Add(key, v)
		}
	}
}
