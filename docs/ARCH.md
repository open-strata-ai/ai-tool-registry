# ai-tool-registry · Architecture (Architecture Overview)

> **Excerpted from** `docs/DESIGN.md` §1 Positioning and Boundaries · §2 List of Responsibilities · §3 Core Interface · §6 Adapter
> **Language · Framework**: Go · Gin + Cobra + Wire (DDD four layers; hot path can be Hertz/go-zero)
> **Field**: agent-infra (Agent infrastructure layer · Tool registration center)
> **optional**: false (core · core; the core of the Agent calling tool)
> **Platform version**: v1.0.0

---

## §1 Positioning and Boundary (Scope)

### 1.1 Positioning in one sentence

`ai-tool-registry` is the **tool registry** of OpenStrata, hosting the architecture §4.3.2 "Tool Registration Center" and §7 "Skills / Rules / Specs Management Platform". It uniformly manages the registration, Schema verification, service discovery, authentication and call metering of Tools and Skills/Rules/Specs that can be called by Agent. It is the only management surface for Agent to call external capabilities through `ToolRegistry` SPI.

### 1.2 Core Problems Solved

Converge "tools (DB/API/file/code/search/business) scattered everywhere" and "platform-level capability packages (Skill/Rule/Spec)" into a registry that can be declared, verified, discovered, and managed, and can be uniformly accessed by `tool_bindings` (§4.3.5) when the Agent is running.

### 1.3 Requirement and Applicable Scenarios

- **Required**: core (recommended, §10.2 "Tool Registration Center")
- **Minimum Scenario**: This repository is indispensable when Agent needs to call any tool
- **Omitable scenario**: Pure API gateway scenario (no Agent calling tool required)
- **Standard deployment**: enabled by default, matched with `ai-gateway-core`

### 1.4 Architecture role

| Dimensions | Description |
| --- | --- |
| **Level** | DDD four layers: `domain/` (ToolRegistry Port + capability package entity) · `application/` (registration/parsing/verification use case) · `infrastructure/` (adapter/DI) · `interfaces/` (Gin handler) |
| **Management plane framework** | Gin (Tool registration/query/management API) |
| **Hot Path Framework** | Hertz/go-zero (optional, `Resolve` is the hot path, Agent must pass through each tool call) |
| **DI solution** | Wire (compile-time dependency injection) |
| **Protocol Support** | MCP (Model Context Protocol): stdio/SSE/HTT three Transports |

### 1.5 Division of labor with other Go components

| Component | Relationship Type | Description |
| --- | --- | --- | --- |
| `ai-gateway-core` | Request link series | The gateway is responsible for the "model calling" data side; the repository is responsible for the "tool calling" management side. Agent debugging tool → own repository analysis/authentication/measurement → tool implementation (possibly debugging the model through the gateway). |
| `ai-sandbox-manager` | Indirect dependency | After the code class Tool (`kind=code`) is registered with this repository, the runtime is hosted by `SandboxExecutor` (§10.6 Dependency rule `Tool→SandboxExecutor`). This repository does not execute code, only registration and routing. |
| `ai-platform-api` | Control plane layering | The control plane does tenant/user-level authorization summary; this repository does tool-level RBAC and instance discovery. |
| `ai-cli` | client/server | `aictl` registers/queries tools and capability packages through this repository API (`aictl registry push`). |

### 1.6 Boundary constraints

| Constraints | Description |
| --- | --- |
| **Do not execute the tool** | Only registration, discovery, routing, and verification are performed, and the tool is executed on the actual instance |
| **Does not execute code** | The code class Tool runtime is hosted by `ai-sandbox-manager` |
| **No model calling** | Model calling is the responsibility of `ai-gateway-core` |
| **No tenant settlement** | Metering data is reported to `ai-billing-service`, which is responsible for settlement |

### 1.7 Upstream dependencies (who calls this service)

`ai-tool-registry` is a **sink** for registration and a **source** for `Resolve`. The
services below call into it. (The Go peer components in §1.5 — `ai-gateway-core`,
`ai-sandbox-manager`, `ai-platform-api` — are also listed here where they are *writers*.)

| Upstream | Interface / port | Direction | What it does | Canonical? |
| --- | --- | --- | --- | --- |
| **Agent runtime** (Agent Engine) | `ToolRegistry` SPI → `POST /v1/tools/{name}/resolve` | **Read** (hot path) | Resolves a tool/skill at call time; does **not** author capability packages | n/a (read-only) |
| **`ai-srs-service`** | `SkillRegistryPort` (REST, MCP Tool Schema) | **Write** | Broadcasts published `Skill` / `Rule` / `Spec` packages into the registry on the `SkillPublished` event (§7.2–7.4) | **Canonical writer of Skill/Rule/Spec** |
| **`ai-platform-api`** | `AppRegistryPort` (REST fan-out) | **Write** | Registers `Application` ↔ Tool bindings; `ApplicationRegistered` event fans out to this registry | Canonical for Application-owned tools |
| **`ai-cli` (`aictl`)** | Registry REST API | **Write + Read** | `aictl registry push` / query for admin & CI pipelines | Canonical for manual / CI registration |
| **`ai-sdk-go` / `ai-sdk-java`** | `Tool` abstraction → `ToolRegistry` SPI | **Write + Read** | SDK users register custom Tools (MCP stdio/SSE/HTTP) to the platform registry | Canonical for SDK-authored tools |
| **`ai-dependency-resolver`** | Assembly graph only | **None** (build-time reference) | Uses this repo as an *assembly unit* to analyze dependency rules (§10.6); never calls its runtime | n/a |

> **Reciprocity note**: `ai-srs-service/docs/ARCH.md` §4.3.2 / `DESIGN.md` §7.2 already
> document this relationship from the SRS side (`SkillRegistryPort` → `ai-tool-registry`).
> The table above is the mirror view, owned by this repository.

**Canonical write path for capability packages (resolves the §14 open question #1):**

- `Skill` / `Rule` / `Spec` packages are *authored and versioned* in `ai-srs-service`.
  The **canonical (and only) writer** of these into `ai-tool-registry` is
  `ai-srs-service`'s `SkillRegistryPort.broadcast()`, triggered by the `SkillPublished`
  event. `ai-tool-registry` is a **passive sink** for them — it never originates a
  capability package.
- **Agent-resolve is a read path only** for capability packages: the Agent calls
  `Resolve` to look up a skill at call time; it does **not** publish/author
  Skill/Rule/Spec. (The SRS sequence diagram's `AGENT->>TR: Registration tool` step
  refers to the Agent registering a *plain Tool* it owns via the `ToolRegistry`
  `Register` SPI — distinct from the capability-package broadcast. Both are valid but
  target different entity kinds: Tools vs Skill/Rule/Spec.)
- **Relationship to §10.6 Component Registry**: Skills/Rules/Specs are *managed within*
  `ai-tool-registry` (the single management surface, §1.1) and *referenced* by the
  Component Registry / `AgentSpec` via `skill_ref` / `rule_ref` / `spec_ref` pointers
  (semantic-version resolved at bind time, §5.4). They are **not** duplicated into
  Component Registry instance metadata → managed here, referenced elsewhere.

---

## §2 Responsibilities List

### 2.1 Complete list of responsibilities

| # | Responsibilities | Required/Optional | Trigger conditions | Description |
| --- | --- | --- | --- | --- |
| R1 | **Tool registration/unregistration** | core | Administrator/CI registration through API | YAML/JSON declaration tool Schema, automatically generate call contract (§4.3.2) |
| R2 | **Schema verification** | core (recommended) | before/after tool call | input/output JSON Schema verification, use gojsonschema (§4.3.2) |
| R3 | **Service Discovery** | optional | `discovery.enabled: true` | Automatic registration of tool instances, heartbeat detection, load balancing |
| R4 | **Tool Authentication (RBAC)** | optional (can be skipped by single user) | When `Resolve`/`invoke` | Tool-level API Key / OAuth2 (§4.3.2) |
| R5 | **Call metering** | optional | `metering.enabled: true` | Number of calls, delay, success rate statistics (§4.3.2) |
| R6 | **Skills Management** | optional (default off) | `skills.enabled: true` | Skill pack registration/version/binding (§7.2) |
| R7 | **Rules Management** | optional (default off) | `rules.enabled: true` | OPA/Rego rule package registration and sandbox execution (§7.3) |
| R8 | **Specs Management** | optional (default off) | when `specs.enabled: true` | AgentSpec template/spec registration (§7.4, §4.3.5) |
| R9 | **MCP protocol access** | Registered with the tool | According to the `transport` field | stdio / SSE / HTTP three Transport adaptation (§4.3.2) |

### 2.2 Responsibility classification

| Level | Responsibility Number | Quantity | Description |
| --- | --- | --- | --- |
| **core (cannot be closed)** | R1, R2, R9 | 3 | Minimum feasible set of tool registry |
| **Default on (configurable off)** | R3, R4, R5 | 3 | Service discovery, authentication, metering |
| **Default off (optional)** | R6, R7, R8 | 3 | Capability package management (advanced scenario) |

### 2.3 Enable policy

| Configuration items | Default values ​​| Enable effects | Turn off effects |
| --- | --- | --- | --- |
| `schemaValidation` | `true` | Mandatory Schema verification for all tool calls | No verification, trust tool implementation (not recommended) |
| `discovery.enabled` | `true` | Multi-instance load balancing (weighted_random) | Only use `ToolDefinition.Endpoint` single point |
| `metering.enabled` | `true` | Call statistics (number of times/latency/success rate) | No metering data |
| `skills.enabled` | `false` | Skill pack registration and version management | Unable to reference skill pack |
| `rules.enabled` | `false` | OPA/Rego rule execution | No rule capability |
| `specs.enabled` | `false` | AgentSpec template management | No AgentSpec template |

---

## §3 Core interface and abstraction

### 3.1 Design principles

The domain layer (`domain/`) defines the `ToolRegistry` Port and capability package entities. `ToolRegistry` has no corresponding external instance** among the 15 SPI ports in bom.yaml (it is a self-developed port on the platform). Its "multiple implementations" are reflected in the coexistence of multiple instances of the same tool (service discovery), rather than external component replacement (§10.3, §10.4).

### 3.2 ToolRegistry SPI

```go
// ===== ToolRegistry SPI（§10.3）=====
package domain

type ToolRegistry interface {
    Register(ctx context.Context, def ToolDefinition) (ToolID, error)
    Deregister(ctx context.Context, id ToolID) error
    Resolve(ctx context.Context, name string, tenantID string) (ResolvedTool, error)
    List(ctx context.Context, tenantID string, filter ToolFilter) []ToolDefinition
    Validate(ctx context.Context, name string, input map[string]any) error
}

type ToolDefinition struct {
    Name         string            `json:"name"`          //Unique, such as order_lookup
    Version      string            `json:"version"`
    Kind         string            `json:"kind"`          // db|api|file|code|search|business
    TenantID     string            `json:"tenant_id"`
    InputSchema  map[string]any    `json:"input_schema"`  // JSON Schema
    OutputSchema map[string]any    `json:"output_schema"`
    Transport    string            `json:"transport"`     // stdio|sse|http|local
    Endpoint     string            `json:"endpoint"`      //Instance address (populated by service discovery)
    Auth         ToolAuth          `json:"auth"`          // apikey|oauth2|none
    RBAC         []string          `json:"rbac"`          //Allowed role list
    CapabilityTags []string        `json:"tags"`          //Semantic search tags
}

type ResolvedTool struct {
    Def       ToolDefinition
    Instances []ToolInstance       //Multiple instances obtained by service discovery (load balancing)
}

type ToolInstance struct {
    Endpoint string
    Healthy  bool
    Weight   int
}

type ToolAuth struct {
    Type   string         `json:"type"`   // apikey|oauth2|none
    Config map[string]any `json:"config"`
}

type ToolFilter struct {
    Kind  string   // db|api|file|code|search|business
    Tags  []string
    Limit int
}

type ToolID string // unique identifier
```

### 3.3 Capability package entity

```go
//===== Capability Package Entity (§7) =====
type Skill struct {
    ID       string            `json:"id"`
    Name     string            `json:"name"`
    Version  string            `json:"version"`
    Manifest map[string]any    `json:"manifest"`
    TenantID string            `json:"tenant_id"`
    CreatedAt time.Time        `json:"created_at"`
}

type Rule struct {
    ID         string `json:"id"`
    Name       string `json:"name"`
    PolicyRego string `json:"policy_rego"` //OPA/Rego full policy text
    Severity   string `json:"severity"`    // low|medium|high|critical
    TenantID   string `json:"tenant_id"`
    CreatedAt  time.Time `json:"created_at"`
}

type Spec struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    AgentSpecRef string `json:"agent_spec_ref"` //Quote §4.3.5 AgentSpec
    TenantID     string `json:"tenant_id"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### 3.4 Delay budget

| Operations | Budget | Description |
| --- | --- | --- |
| `Resolve` (local cache hit) | ≤5ms (p99) | `sync.RWMutex` protects local map reads |
| `Resolve` (Redis back to origin) | ≤10ms (p99) | When local cache misses |
| `Schema verification` | ≤2ms | gojsonschema single verification |
| Service discovery (Redis) | ≤3ms | Get healthy instance list |
| Agent call (`invoke`) | Depend on downstream tools | Context timeout control, default 30s |

### 3.5 Summary of calling paths

```
Agent runtime → Resolve(name, tenant) → Tenant context + RBAC check
  → Schema Verify input parameters → Service discovery gets healthy instances → loadBalance Select instance
  → MCP Transport Tuning tool implementation(stdio/SSE/HTTP) → call metering/Parameter verification → return
```

> The registration path of the capability package (Skill/Rule/Spec) is "Declaration → Verification → Into the library → Can be referenced by AgentSpec", and does not use the runtime call link (§7).

---

## §6 Adapter and SPI Ecosystem

### 6.1 SPI port matrix

| SPI port | Role of this repository | External component (bom.yaml) | Default ✅ / Alternative | Adapter |
| --- | --- | --- | --- | --- |
| `ToolRegistry` | Implementer | This repository itself (no external SPI instance, self-developed port of the platform) | — | MCP protocol access (stdio/SSE/HTTP) |
| `VectorStore` (1.0.0) | Consumer (optional) | Qdrant (core) / Milvus (optional) | ✅ / Alternative | Tool semantic retrieval/tag vectorization (optional enhancement) |
| `Cache` (1.0.0) | Consumer | Redis (core) / Valkey (optional) | ✅ / Alternative | Registry hot copy, metering count |
| `Auth` (1.0.0) | Consumer | Keycloak (core) | ✅ | Tenant/User JWT Identity Verification |
| `Sandbox` (1.0.0) | Indirect dependency | Kata/E2B (optional) | Alternative | Code class Tool runtime is hosted by `SandboxExecutor` |

### 6.2 MCP Transport Adapter

| Transport | Protocol characteristics | Applicable scenarios | Adapter key processing |
| --- | --- | --- | --- |
| `stdio` | Standard input/output, sub-process communication | Local CLI tools, Python/Node scripts | Process startup management + stdin/stdout pipe + process life cycle |
| `SSE` | Server-Sent Events one-way flow | streaming tools, real-time push | HTTP long connection management + event stream analysis + disconnection and reconnection |
| `HTTP` | REST / gRPC standard calls | Remote microservice tools | Standard HTTP client + JSON serialization + connection pool |

### 6.3 Anti-corrosion layer (ACL)

The differences between the three MCP Transports are converged into a unified `ResolvedTool` call within the Adapter. The caller does not need to be aware of the underlying transport differences:

- `stdio` Adapter: child process management, automatic reconnection
- `SSE` Adapter: long connection event stream → internal result channel
- `HTTP` Adapter: standard REST → internal response struct

### 6.4 ToolRegistry multiple implementation strategies

`ToolRegistry` has no corresponding external instance** among the 15 SPI ports in bom.yaml (it is a self-developed port on the platform). Its "multiple realizations" are reflected in:

1. **The same tool name supports multiple versions**: `name+version` immutable storage, AgentSpec is referenced according to the semantic version
2. **The same version supports multiple instances**: Service discovery returns `[]ToolInstance`, weighted random/polling by `Weight`
3. **Hot copy mechanism**: Redis master copy + local `sync.RWMutex` protection map, `Resolve` p99 ≤ 5ms

### 6.5 Registry caching strategy

| Cache Layer | Storage | Read Latency | Consistency |
| --- | --- | --- | --- |
| L1 (local) | `sync.RWMutex` protected map | ≤1ms | Eventually consistent (double-write failure) |
| L2 (Redis) | Hash (JSON) | ≤3ms | Strong consistency (write through PG) |
| Authoritative | PostgreSQL | ≤10ms | Strong consistency |

Double writing process: Register tool → Write PG (authoritative) → Delete Redis hot copy → Invalidate local cache. `Resolve` reads L1 first, misses back to the source L2, then checks PG and backfills it.

### 6.6 Indirect dependency: SandboxExecutor binding

After the code class Tool (`kind=code`) is registered with this repository, the runtime is hosted by the `SandboxExecutor` of `ai-sandbox-manager`:

```go
//Dependency rules declared in TenantCode (§10.6)
// Tool(ai-tool-registry) → SandboxExecutor(ai-sandbox-manager)
```

- Declaring `kind: code` when registering does not force `ai-sandbox-manager` to be online (weak dependency)
- But if the sandbox is not available when calling, `503 Sandbox Unavailable` will be returned.
- `ai-provisioning-engine` detects this dependency rule and prompts during assembly

### 6.7 Optional enhancement components

| Component | Enabling Conditions | Function | Implementation |
| --- | --- | --- | --- |
| VectorStore (Qdrant/Milvus) | `vectorSearch.enabled: true` | Tool semantic retrieval, label vectorization | Tool label → Embedding → VectorStore index |
| SandboxExecutor | Code class Tool registration + when called | Code sandbox execution | Via `ai-sandbox-manager` Sandbox SPI |

---

## Request path panorama

### Tool runtime call (hot path)

```
Agent runtime / ai-platform-api
  → POST /v1/tools/{name}/resolve (JWT)
    → ai-tool-registry access layer handler [Gin / Hertz]
      → Tenant context resolution（Keycloak JWT → tenant_id / role）
        → Tool level RBAC check（ToolDefinition.RBAC vs caller role）
          → [No rights] → 403 Forbidden + audit
          → [pass] → Schema Verify input parameters（gojsonschema）
            → [illegal] → 400 Bad Request + Error details
            → [legitimate] → service discovery: Take healthy example（Redis hot copy）
              → Load balancing instance selection（weighted_random / round_robin）
                → through MCP Transport Tuning tool implementation（stdio/SSE/HTTP）
                  → call metering: frequency/Delay（Asynchronous writing tool_metrics）
                    → Take out the ginseng Schema check（Alarms are not blocked）
                      → return ResolvedTool + audit

Capability package registration path（Management aspect）:
statement → JSON Schema check → Repository PG → can be AgentSpec Quote（§7）
```

---

> **Associated documents**: This repository `docs/DESIGN.md` · `docs/SKILLS.md` · `docs/SPECS.md`
> **Architecture Reference**: §4.3.2 (Tool Registry) · §7 (Skills/Rules/Specs Management) · §4.3.5 (AgentSpec Tool Binding) · §10.3 (ToolRegistry SPI) · §10.6 (Component Registry) · §15.5 (DDD Layering) · §16 (BOM)
