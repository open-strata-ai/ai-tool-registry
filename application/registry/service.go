// Package registry is the central use case (DESIGN §4 request path): registration
// → schema validation → RBAC → service discovery → (invoke) transport call →
// metering + audit. It composes the domain ports with the infrastructure adapters.
package registry

import (
	"context"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/open-strata-ai/ai-tool-registry/application/discovery"
	"github.com/open-strata-ai/ai-tool-registry/application/rbac"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Allowed enumerations (DESIGN §3.2 / SPECS §8.2).
var (
	allowedKinds      = []string{domain.KindDB, domain.KindAPI, domain.KindFile, domain.KindCode, domain.KindSearch, domain.KindBusiness}
	allowedTransports = []string{domain.TransportStdio, domain.TransportSSE, domain.TransportHTTP, domain.TransportLocal}
	allowedAuthTypes  = []string{domain.AuthNone, domain.AuthAPIKey, domain.AuthOAuth2}
)

type ctxKeyRole struct{}
type ctxKeyTenant struct{}

// WithRole attaches the caller role to the context for RBAC checks in Resolve.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxKeyRole{}, role)
}

// WithTenant attaches the resolved tenant to the context. Validate's SPI
// signature carries only the tool name, so the tenant is supplied via context.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKeyTenant{}, tenantID)
}

func roleFromCtx(ctx context.Context) string {
	if r, ok := ctx.Value(ctxKeyRole{}).(string); ok {
		return r
	}
	return ""
}

func tenantFromCtx(ctx context.Context) string {
	if t, ok := ctx.Value(ctxKeyTenant{}).(string); ok {
		return t
	}
	return ""
}

// TransportSelector returns the Transport adapter for a transport kind.
type TransportSelector func(kind string) (domain.Transport, bool)

// Deps are the collaborators of the Service (constructor-injected).
type Deps struct {
	Store      domain.Store
	Discovery  domain.Discovery
	Schema     domain.SchemaValidator
	Metering   domain.MeteringPort
	Auth       domain.AuthPort
	RBAC       *rbac.Checker
	Transports TransportSelector
}

// Config holds tunables for the Service.
type Config struct {
	SchemaValidation bool
	DiscoveryEnabled bool
}

// Service orchestrates tool registration and resolution.
type Service struct {
	d       Deps
	conf    Config
	checker *rbac.Checker
	rng     *rand.Rand
	mu      sync.Mutex
	now     func() time.Time
}

// New builds a Service.
func New(d Deps, c Config) *Service {
	checker := d.RBAC
	if checker == nil {
		checker = rbac.New()
	}
	if d.Schema == nil {
		d.Schema = schemaNoop{}
	}
	return &Service{
		d:       d,
		conf:    c,
		checker: checker,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		now:     time.Now,
	}
}

// Register validates and persists a tool definition (R1 / DESIGN §5).
func (s *Service) Register(ctx context.Context, def domain.ToolDefinition) (domain.ToolID, error) {
	if def.Name == "" || def.Version == "" || def.TenantID == "" {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"name, version and tenant_id are required")
	}
	if def.Auth.Type == "" {
		def.Auth.Type = domain.AuthNone
	}
	if !contains(allowedKinds, def.Kind) {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"invalid kind "+def.Kind)
	}
	if !contains(allowedTransports, def.Transport) {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"invalid transport "+def.Transport)
	}
	if !contains(allowedAuthTypes, def.Auth.Type) {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"invalid auth type "+def.Auth.Type)
	}
	if s.conf.SchemaValidation {
		// input_schema is a typed map[string]any; its structural validation
		// against actual call input happens in Validate/Invoke (R2 / §5.2).
	}
	if err := s.d.Store.SaveTool(def); err != nil {
		return "", domain.NewError(domain.ErrUpstream, http.StatusInternalServerError, err.Error())
	}
	return domain.ToolKey(def.TenantID, def.Name, def.Version), nil
}

// Deregister removes a tool by its composite ToolID (R1).
func (s *Service) Deregister(ctx context.Context, id domain.ToolID) error {
	tenant, name, ok := splitKey(string(id))
	if !ok {
		return domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "malformed tool id")
	}
	if err := s.d.Store.DeleteTool(tenant, name); err != nil {
		return domain.NewError(domain.ErrNotFound, http.StatusNotFound, "tool not registered")
	}
	return nil
}

// Resolve returns the definition plus healthy discovered instances (R2/R3/§5.1).
func (s *Service) Resolve(ctx context.Context, name, tenantID string) (domain.ResolvedTool, error) {
	def, ok := s.d.Store.GetLatestTool(tenantID, name)
	if !ok {
		return domain.ResolvedTool{}, domain.NewError(domain.ErrNotFound, http.StatusNotFound,
			"tool "+name+" not registered for tenant "+tenantID)
	}
	if !s.checker.Allow(def, roleFromCtx(ctx)) {
		return domain.ResolvedTool{}, domain.NewError(domain.ErrForbidden, http.StatusForbidden,
			"role not permitted by tool RBAC")
	}
	instances := s.resolveInstances(def)
	return domain.ResolvedTool{Def: def, Instances: instances}, nil
}

func (s *Service) resolveInstances(def domain.ToolDefinition) []domain.ToolInstance {
	toolID := domain.ToolKey(def.TenantID, def.Name, def.Version)
	if s.conf.DiscoveryEnabled && s.d.Discovery != nil {
		healthy := discovery.Healthy(s.d.Discovery.Instances(toolID))
		if len(healthy) > 0 {
			return healthy
		}
	}
	if def.Endpoint != "" {
		return []domain.ToolInstance{{Endpoint: def.Endpoint, Healthy: true, Weight: 1}}
	}
	return nil
}

// List returns the tenant's tool definitions, optionally filtered (SPECS §7.4).
func (s *Service) List(ctx context.Context, tenantID string, filter domain.ToolFilter) []domain.ToolDefinition {
	return s.d.Store.ListTools(tenantID, filter)
}

// Validate checks input against the tool's input_schema (R2 / §5.2).
func (s *Service) Validate(ctx context.Context, name string, input map[string]any) error {
	def, ok := s.d.Store.GetLatestTool(tenantFromCtx(ctx), name)
	if !ok {
		return domain.NewError(domain.ErrNotFound, http.StatusNotFound, "tool not registered")
	}
	if !s.conf.SchemaValidation || s.d.Schema == nil {
		return nil
	}
	if err := s.d.Schema.Validate(def.InputSchema, input); err != nil {
		return domain.NewError(domain.ErrSchemaInvalid, http.StatusBadRequest, err.Error())
	}
	return nil
}

// Invoke resolves the tool, checks RBAC, calls the selected instance over its
// MCP transport, and records metering (R5 / DESIGN §4). The registry never
// executes code itself — only routes to the instance (§1.6).
func (s *Service) Invoke(ctx context.Context, name, tenantID string, input map[string]any) (map[string]any, error) {
	resolved, err := s.Resolve(ctx, name, tenantID)
	if err != nil {
		return nil, err
	}
	if s.conf.SchemaValidation {
		if err := s.d.Schema.Validate(resolved.Def.InputSchema, input); err != nil {
			return nil, domain.NewError(domain.ErrSchemaInvalid, http.StatusBadRequest, err.Error())
		}
	}
	inst, ok := s.pick(resolved.Instances)
	if !ok {
		return nil, domain.NewError(domain.ErrUpstream, http.StatusBadGateway, "no healthy instance")
	}
	tr, ok := s.d.Transports(resolved.Def.Transport)
	if !ok {
		return nil, domain.NewError(domain.ErrUpstream, http.StatusBadGateway,
			"no transport for "+resolved.Def.Transport)
	}
	start := s.now()
	out, callErr := tr.Invoke(ctx, inst, input)
	latency := s.now().Sub(start).Milliseconds()
	s.record(tenantID, name, latency, callErr == nil)
	if callErr != nil {
		return nil, domain.NewError(domain.ErrUpstream, http.StatusBadGateway, callErr.Error())
	}
	return out, nil
}

func (s *Service) pick(insts []domain.ToolInstance) (domain.ToolInstance, bool) {
	s.mu.Lock()
	r := s.rng.Float64()
	s.mu.Unlock()
	return discovery.Pick(insts, func() float64 { return r })
}

func (s *Service) record(tenant, name string, latencyMs int64, success bool) {
	if s.d.Metering == nil {
		return
	}
	s.d.Metering.Record(domain.CallMetric{
		TenantID:  tenant,
		ToolName:  name,
		LatencyMs: latencyMs,
		Success:   success,
	})
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func splitKey(id string) (tenant, name string, ok bool) {
	for i := 0; i < len(id); i++ {
		if id[i] == '/' {
			tenant = id[:i]
			rest := id[i+1:]
			for j := 0; j < len(rest); j++ {
				if rest[j] == '/' {
					return tenant, rest[:j], true
				}
			}
			return tenant, rest, true
		}
	}
	return "", "", false
}

// --- Capability packages (§7) ---

// RegisterSkill stores a skill package (§7.2).
func (s *Service) RegisterSkill(ctx context.Context, sk domain.Skill) (string, error) {
	if sk.Name == "" || sk.Version == "" || sk.TenantID == "" {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"name, version and tenant_id are required")
	}
	if sk.ID == "" {
		sk.ID = sk.TenantID + "/" + sk.Name + "/" + sk.Version
	}
	if err := s.d.Store.SaveSkill(sk); err != nil {
		return "", domain.NewError(domain.ErrUpstream, http.StatusInternalServerError, err.Error())
	}
	return sk.ID, nil
}

// ListSkills returns skill packages for a tenant, optionally by version (§7.2).
func (s *Service) ListSkills(ctx context.Context, tenantID, version string) []domain.Skill {
	return s.d.Store.ListSkills(tenantID, version)
}

// RegisterRule stores an OPA/Rego rule package (§7.3).
func (s *Service) RegisterRule(ctx context.Context, r domain.Rule) (string, error) {
	if r.Name == "" || r.TenantID == "" {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"name and tenant_id are required")
	}
	if r.ID == "" {
		r.ID = r.TenantID + "/" + r.Name
	}
	if err := s.d.Store.SaveRule(r); err != nil {
		return "", domain.NewError(domain.ErrUpstream, http.StatusInternalServerError, err.Error())
	}
	return r.ID, nil
}

// ListRules returns rule packages for a tenant (§7.3).
func (s *Service) ListRules(ctx context.Context, tenantID string) []domain.Rule {
	return s.d.Store.ListRules(tenantID)
}

// RegisterSpec stores an AgentSpec template reference (§7.4).
func (s *Service) RegisterSpec(ctx context.Context, sp domain.Spec) (string, error) {
	if sp.Name == "" || sp.TenantID == "" {
		return "", domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest,
			"name and tenant_id are required")
	}
	if sp.ID == "" {
		sp.ID = sp.TenantID + "/" + sp.Name
	}
	if err := s.d.Store.SaveSpec(sp); err != nil {
		return "", domain.NewError(domain.ErrUpstream, http.StatusInternalServerError, err.Error())
	}
	return sp.ID, nil
}

// ListSpecs returns spec packages for a tenant (§7.4).
func (s *Service) ListSpecs(ctx context.Context, tenantID string) []domain.Spec {
	return s.d.Store.ListSpecs(tenantID)
}

// schemaNoop passes all validation when no validator is wired.
type schemaNoop struct{}

func (schemaNoop) Validate(map[string]any, map[string]any) error { return nil }
