// Package mcp implements the MCP transport anti-corrosion layer (DESIGN §6.2 /
// §6.3): each adapter normalizes one transport (http/stdio/sse/local) to a
// uniform domain.Transport call. The registry routes; it does not execute code
// itself (§1.6) — stdio/SSE runtimes are hosted by ai-sandbox-manager.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// HTTPTransport invokes a tool over plain REST/JSON (transport=http, §6.2).
type HTTPTransport struct {
	Client *http.Client
}

// NewHTTP builds an HTTP transport; a nil client defaults to http.DefaultClient.
func NewHTTP(client *http.Client) *HTTPTransport {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{Client: client}
}

func (t *HTTPTransport) Kind() string { return domain.TransportHTTP }

func (t *HTTPTransport) Invoke(ctx context.Context, inst domain.ToolInstance, input map[string]any) (map[string]any, error) {
	body, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tool returned status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// NotImplementedTransport is the stdio/SSE/local stand-in. Those runtimes are
// delegated to the tool's own process / streaming MCP server / sandbox (§1.6).
type NotImplementedTransport struct{ kind string }

// NewStdio / NewSSE / NewLocal build the delegated stand-ins.
func NewStdio() *NotImplementedTransport { return &NotImplementedTransport{kind: domain.TransportStdio} }
func NewSSE() *NotImplementedTransport   { return &NotImplementedTransport{kind: domain.TransportSSE} }
func NewLocal() *NotImplementedTransport { return &NotImplementedTransport{kind: domain.TransportLocal} }

func (t *NotImplementedTransport) Kind() string { return t.kind }

func (t *NotImplementedTransport) Invoke(ctx context.Context, inst domain.ToolInstance, input map[string]any) (map[string]any, error) {
	return nil, fmt.Errorf("%s transport invocation is delegated to the tool runtime (not implemented in registry)", t.kind)
}

// Selector returns a TransportSelector choosing the adapter by transport kind.
func Selector(httpClient *http.Client) func(string) (domain.Transport, bool) {
	h := NewHTTP(httpClient)
	stdio := NewStdio()
	sse := NewSSE()
	local := NewLocal()
	return func(kind string) (domain.Transport, bool) {
		switch kind {
		case domain.TransportHTTP:
			return h, true
		case domain.TransportStdio:
			return stdio, true
		case domain.TransportSSE:
			return sse, true
		case domain.TransportLocal:
			return local, true
		}
		return nil, false
	}
}

var (
	_ domain.Transport = (*HTTPTransport)(nil)
	_ domain.Transport = (*NotImplementedTransport)(nil)
)
