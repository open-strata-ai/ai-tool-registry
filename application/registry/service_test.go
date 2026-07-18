package registry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
)

type fixture struct {
	svc   *registry.Service
	store *store.InMemory
	disc  *discovery.InMemory
	rec   *metering.Recorder
}

func newFixture() *fixture {
	st := store.New()
	disc := discovery.New()
	rec := metering.New(64, nil)
	svc := registry.New(registry.Deps{
		Store:      st,
		Discovery:  disc,
		Schema:     schema.New(),
		Metering:   rec,
		Auth:       auth.New(""),
		RBAC:       rbac.New(),
		Transports: mcp.Selector(nil),
	}, registry.Config{SchemaValidation: true, DiscoveryEnabled: true})
	return &fixture{svc: svc, store: st, disc: disc, rec: rec}
}

func sampleTool() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:      "order_lookup",
		Version:   "1.0.0",
		Kind:      domain.KindAPI,
		TenantID:  "tenant-A",
		Transport: domain.TransportHTTP,
		Endpoint:  "http://order-svc:8080/lookup",
		RBAC:      []string{"agent"},
		InputSchema: map[string]any{
			"type":       "object",
			"required":   []any{"order_id"},
			"properties": map[string]any{"order_id": map[string]any{"type": "string"}},
		},
	}
}

func TestRegisterAndResolve(t *testing.T) {
	f := newFixture()
	id, err := f.svc.Register(context.Background(), sampleTool())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if id == "" {
		t.Fatalf("expected tool id")
	}
	ctx := registry.WithRole(context.Background(), "agent")
	resolved, err := f.svc.Resolve(ctx, "order_lookup", "tenant-A")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Def.Name != "order_lookup" {
		t.Fatalf("want order_lookup, got %s", resolved.Def.Name)
	}
	if len(resolved.Instances) != 1 || !resolved.Instances[0].Healthy {
		t.Fatalf("expected single healthy fallback instance, got %+v", resolved.Instances)
	}
}

func TestResolveForbiddenByRBAC(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.Register(context.Background(), sampleTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := registry.WithRole(context.Background(), "member")
	_, err := f.svc.Resolve(ctx, "order_lookup", "tenant-A")
	re, ok := err.(*domain.RegistryError)
	if !ok || re.Code != domain.ErrForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestResolveAdminBypassesRBAC(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.Register(context.Background(), sampleTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := registry.WithRole(context.Background(), "admin")
	if _, err := f.svc.Resolve(ctx, "order_lookup", "tenant-A"); err != nil {
		t.Fatalf("admin should bypass RBAC: %v", err)
	}
}

func TestDeregister(t *testing.T) {
	f := newFixture()
	id, _ := f.svc.Register(context.Background(), sampleTool())
	if err := f.svc.Deregister(context.Background(), id); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	ctx := registry.WithRole(context.Background(), "agent")
	if _, err := f.svc.Resolve(ctx, "order_lookup", "tenant-A"); err == nil {
		t.Fatalf("expected not found after deregister")
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.Register(context.Background(), sampleTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := registry.WithTenant(context.Background(), "tenant-A")
	err := f.svc.Validate(ctx, "order_lookup", map[string]any{})
	re, ok := err.(*domain.RegistryError)
	if !ok || re.Code != domain.ErrSchemaInvalid {
		t.Fatalf("want schema_invalid, got %v", err)
	}
	if err := f.svc.Validate(ctx, "order_lookup", map[string]any{"order_id": "O123"}); err != nil {
		t.Fatalf("valid input should pass: %v", err)
	}
}

func TestInvokeCallsHTTPTransport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"shipped","amount":42}`))
	}))
	defer ts.Close()

	f := newFixture()
	tool := sampleTool()
	tool.Endpoint = ts.URL
	if _, err := f.svc.Register(context.Background(), tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := registry.WithRole(context.Background(), "agent")
	out, err := f.svc.Invoke(ctx, "order_lookup", "tenant-A", map[string]any{"order_id": "O123"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out["status"] != "shipped" {
		t.Fatalf("unexpected invoke result: %v", out)
	}
}

func TestInvokeRecordsMetering(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	f := newFixture()
	tool := sampleTool()
	tool.Endpoint = ts.URL
	_, _ = f.svc.Register(context.Background(), tool)
	ctx := registry.WithRole(context.Background(), "agent")
	if _, err := f.svc.Invoke(ctx, "order_lookup", "tenant-A", map[string]any{"order_id": "O1"}); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	f.rec.Close()
	var found bool
	for _, m := range f.rec.Snapshot() {
		if m.ToolName == "order_lookup" && m.Calls == 1 && m.Success == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected metering record for order_lookup, got %+v", f.rec.Snapshot())
	}
}

func TestListFiltersByKind(t *testing.T) {
	f := newFixture()
	_ = f.store.SaveTool(sampleTool())
	_ = f.store.SaveTool(domain.ToolDefinition{
		Name: "db_search", Version: "1.0.0", Kind: domain.KindDB, TenantID: "tenant-A",
		Transport: domain.TransportLocal, Endpoint: "x",
	})
	tools := f.svc.List(context.Background(), "tenant-A", domain.ToolFilter{Kind: domain.KindDB})
	if len(tools) != 1 || tools[0].Name != "db_search" {
		t.Fatalf("want 1 db tool, got %+v", tools)
	}
}
