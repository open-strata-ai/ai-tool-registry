// Package domain holds source-independent value types and Port interfaces for
// ai-tool-registry. It has no external dependencies (DDD domain layer, DESIGN §3).
package domain

import "time"

// Tool kind constants (DESIGN §3.2 / SPECS §8.2).
const (
	KindDB       = "db"
	KindAPI      = "api"
	KindFile     = "file"
	KindCode     = "code"
	KindSearch   = "search"
	KindBusiness = "business"
)

// Transport constants (DESIGN §3.2 / §6.2).
const (
	TransportStdio = "stdio"
	TransportSSE   = "sse"
	TransportHTTP  = "http"
	TransportLocal = "local"
)

// Auth types (DESIGN §3.2).
const (
	AuthNone   = "none"
	AuthAPIKey = "apikey"
	AuthOAuth2 = "oauth2"
)

// ToolID uniquely identifies a tenant-scoped tool registration.
type ToolID string

// ToolKey builds the composite key tenant/name/version used as a ToolID.
func ToolKey(tenantID, name, version string) ToolID {
	return ToolID(tenantID + "/" + name + "/" + version)
}

// ToolAuth is the authentication configuration attached to a tool (DESIGN §3.2).
type ToolAuth struct {
	Type   string         `json:"type"` // none|apikey|oauth2
	Config map[string]any `json:"config,omitempty"`
}

// ToolDefinition is the registry's core entity (DESIGN §3.2 / SPECS §8.2).
type ToolDefinition struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Kind           string         `json:"kind"` // db|api|file|code|search|business
	TenantID       string         `json:"tenant_id"`
	InputSchema    map[string]any `json:"input_schema,omitempty"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
	Transport      string         `json:"transport"` // stdio|sse|http|local
	Endpoint       string         `json:"endpoint"`
	Auth           ToolAuth       `json:"auth"`
	RBAC           []string       `json:"rbac"` // allowed roles; empty = all
	CapabilityTags []string       `json:"tags,omitempty"`
}

// ToolInstance is a discovered runtime endpoint for a tool (DESIGN §3.2).
type ToolInstance struct {
	Endpoint string `json:"endpoint"`
	Healthy  bool   `json:"healthy"`
	Weight   int    `json:"weight"`
}

// ResolvedTool is the result of Resolve (DESIGN §3.2 / §5.1).
type ResolvedTool struct {
	Def       ToolDefinition `json:"def"`
	Instances []ToolInstance `json:"instances"`
}

// ToolFilter narrows a List query (SPECS §7.4).
type ToolFilter struct {
	Kind  string
	Tags  []string
	Limit int
}

// Skill is a capability package (DESIGN §3.3 / §7.2).
type Skill struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Manifest    map[string]any `json:"manifest,omitempty"`
	TenantID    string         `json:"tenant_id"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Rule is an OPA/Rego capability package (DESIGN §3.3 / §7.3).
type Rule struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	PolicyRego string    `json:"policy_rego"`
	Severity   string    `json:"severity"` // low|medium|high|critical
	TenantID   string    `json:"tenant_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// Spec references an AgentSpec template (DESIGN §3.3 / §7.4).
type Spec struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	AgentSpecRef string    `json:"agent_spec_ref"`
	TenantID     string    `json:"tenant_id"`
	CreatedAt    time.Time `json:"created_at"`
}
