package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/application/metering"
	"github.com/open-strata-ai/ai-tool-registry/application/rbac"
	"github.com/open-strata-ai/ai-tool-registry/application/registry"
	"github.com/open-strata-ai/ai-tool-registry/application/schema"
	"github.com/open-strata-ai/ai-tool-registry/domain"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/auth"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/discovery"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/mcp"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/store"
	httpapi "github.com/open-strata-ai/ai-tool-registry/interfaces/http"
)

func newServer(flags httpapi.FeatureFlags) *httpapi.Handler {
	st := store.New()
	svc := registry.New(registry.Deps{
		Store:      st,
		Discovery:  discovery.New(),
		Schema:     schema.New(),
		Metering:   metering.New(64, nil),
		Auth:       auth.New(""),
		RBAC:       rbac.New(),
		Transports: mcp.Selector(nil),
	}, registry.Config{SchemaValidation: true, DiscoveryEnabled: true})
	return httpapi.New(svc, auth.New(""), flags, nil)
}

const toolBody = `{
  "name":"order_lookup","version":"1.0.0","kind":"api","tenant_id":"tenant-A",
  "transport":"http","endpoint":"http://order-svc:8080/lookup",
  "rbac":["agent"],
  "input_schema":{"type":"object","required":["order_id"],"properties":{"order_id":{"type":"string"}}}
}`

func TestHTTP_RegisterListResolveDeregister(t *testing.T) {
	h := newServer(httpapi.FeatureFlags{})

	// register
	req := httptest.NewRequest(http.MethodPost, "/v1/tools", strings.NewReader(toolBody))
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register want 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	// list
	req = httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list want 200, got %d", rec.Code)
	}

	// resolve as agent
	req = httptest.NewRequest(http.MethodPost, "/v1/tools/order_lookup/resolve", nil)
	req.Header.Set("X-Tenant-Id", "tenant-A")
	req.Header.Set("Authorization", "Bearer tenant-A:agent")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resolved domain.ResolvedTool
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resolved.Def.Name != "order_lookup" {
		t.Fatalf("unexpected resolve body: %s", rec.Body.String())
	}

	// resolve as member → forbidden by RBAC
	req = httptest.NewRequest(http.MethodPost, "/v1/tools/order_lookup/resolve", nil)
	req.Header.Set("X-Tenant-Id", "tenant-A")
	req.Header.Set("Authorization", "Bearer tenant-A:member")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("resolve member want 403, got %d", rec.Code)
	}

	// deregister
	req = httptest.NewRequest(http.MethodDelete, "/v1/tools/order_lookup", nil)
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("deregister want 200, got %d", rec.Code)
	}
}

func TestHTTP_SkillsDisabledByDefault(t *testing.T) {
	h := newServer(httpapi.FeatureFlags{})
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"s","version":"1.0.0"}`))
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("skills disabled want 404, got %d", rec.Code)
	}
}

func TestHTTP_SkillsEnabled(t *testing.T) {
	h := newServer(httpapi.FeatureFlags{Skills: true})
	req := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"s","version":"1.0.0","tenant_id":"tenant-A"}`))
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("skills enabled want 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTP_Healthz(t *testing.T) {
	h := newServer(httpapi.FeatureFlags{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz want 200, got %d", rec.Code)
	}
}

func TestHTTP_InvokeHTTPTransport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	h := newServer(httpapi.FeatureFlags{})
	body := strings.Replace(toolBody, "http://order-svc:8080/lookup", ts.URL, 1)
	req := httptest.NewRequest(http.MethodPost, "/v1/tools", strings.NewReader(body))
	req.Header.Set("X-Tenant-Id", "tenant-A")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register want 201, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/tools/order_lookup/invoke", strings.NewReader(`{"order_id":"O1"}`))
	req.Header.Set("X-Tenant-Id", "tenant-A")
	req.Header.Set("Authorization", "Bearer tenant-A:agent")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("invoke want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected invoke result: %s", rec.Body.String())
	}
}

func TestHTTP_MissingTenantUnauthorized(t *testing.T) {
	h := newServer(httpapi.FeatureFlags{})
	req := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}
