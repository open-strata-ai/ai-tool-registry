package domain

import "context"

// ToolRegistry is the application-facing SPI (DESIGN §3.2 / §10.3). The registry
// service implements it; external components (Agent runtime, ai-platform-api,
// ai-cli) consume it through AgentSpec tool_bindings.
type ToolRegistry interface {
	Register(ctx context.Context, def ToolDefinition) (ToolID, error)
	Deregister(ctx context.Context, id ToolID) error
	Resolve(ctx context.Context, name, tenantID string) (ResolvedTool, error)
	List(ctx context.Context, tenantID string, filter ToolFilter) []ToolDefinition
	Validate(ctx context.Context, name string, input map[string]any) error
}

// Store is the persistence port (DESIGN §8). Offline = in-memory; production =
// PostgreSQL (authoritative) + Redis (L2 hot copy, §6.5).
type Store interface {
	SaveTool(def ToolDefinition) error
	GetTool(tenantID, name, version string) (ToolDefinition, bool)
	GetLatestTool(tenantID, name string) (ToolDefinition, bool)
	DeleteTool(tenantID, name string) error
	ListTools(tenantID string, filter ToolFilter) []ToolDefinition

	SaveSkill(s Skill) error
	GetSkill(tenantID, name, version string) (Skill, bool)
	ListSkills(tenantID, version string) []Skill
	SaveRule(r Rule) error
	GetRule(tenantID, name string) (Rule, bool)
	ListRules(tenantID string) []Rule
	SaveSpec(s Spec) error
	GetSpec(tenantID, name string) (Spec, bool)
	ListSpecs(tenantID string) []Spec
}

// Discovery is the service-discovery port (DESIGN §5.3 / §6.5).
type Discovery interface {
	RegisterInstance(toolID ToolID, inst ToolInstance)
	DeregisterInstance(toolID ToolID, endpoint string)
	Instances(toolID ToolID) []ToolInstance
}

// SchemaValidator verifies input/output against a JSON Schema (DESIGN §5.2).
type SchemaValidator interface {
	Validate(schema map[string]any, data map[string]any) error
}

// MeteringPort records a single tool call metric asynchronously (R5 / §12).
type MeteringPort interface {
	Record(m CallMetric)
}

// CallMetric is one observed tool invocation (R5).
type CallMetric struct {
	TenantID string
	ToolName string
	ToolID   ToolID
	LatencyMs int64
	Success   bool
}

// Transport executes a tool call on a concrete instance (MCP ACL, DESIGN
// §6.2 / §6.3). Each adapter normalizes one MCP transport to a uniform call.
type Transport interface {
	Kind() string
	Invoke(ctx context.Context, inst ToolInstance, input map[string]any) (map[string]any, error)
}

// AuthPort resolves tenant context + role from a request (Auth SPI 1.0.0).
type AuthPort interface {
	Resolve(ctx context.Context, bearer, tenantHeader string) (tenantID, role string, err error)
}

// RoleAllowed reports whether callerRole satisfies a tool's RBAC allow list.
// An empty allow list grants access to every role (DESIGN §5.1).
func RoleAllowed(allowList []string, callerRole string) bool {
	if len(allowList) == 0 {
		return true
	}
	for _, r := range allowList {
		if r == callerRole {
			return true
		}
	}
	return false
}
