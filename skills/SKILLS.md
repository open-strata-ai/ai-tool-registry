# ai-tool-registry · Skills & Rules

> **Skill Rules Layer** — executable rules for key algorithms, concurrency/performance models, and security policies
> **Source document**: design/DESIGN.md §5 / §9 / §12
> **Platform version**: v1.4.0

---

## Algorithm rules (§5)

### RULE-TR-001: Tool parsing and routing

**Trigger**: Call `Resolve(name, tenantID)`

**constraint**:
1. Press `tenant_id + name` to locate `ToolDefinition` (primary key: `tenant_id, name, version`)
2. Verify the caller role according to `RBAC` (no matching role returns 403)
3. Get the healthy instance from `service_discovery`
4. Select instances by instance `Weight` weighted random/round-robin
5. Return `ResolvedTool{Def, Instances}`

**Example**:
```
Input:  Resolve("order_lookup", "tenant-A")
Step1:  PG SELECT WHERE tenant_id='tenant-A' AND name='order_lookup' ORDER BY version DESC LIMIT 1
Step2:  RBAC check: caller role=agent → ["agent"] ∈ ToolDefinition.RBAC → pass
Step3:  Redis SMEMBERS "tool:order_lookup:tenant-A:instances" → [inst1, inst2]
Step4:  Weight=60(inst1), 40(inst2) → weighted random → inst1
Output: ResolvedTool{Def: {...}, Instances: [{Endpoint:"http://inst1:8080", Healthy:true, Weight:60}]}
```

### RULE-TR-002: Schema verification

**Trigger**: before tool invocation (`Validate` or `invoke` path)

**constraint**:
- Use JSON Schema (gojsonschema) to verify input parameters/output parameters
- Illegal input parameters will be intercepted at the border of the registration center and 400 + error details will be returned.
- Verification failures are not passed to tool implementation (to prevent contamination)
- It is recommended to enable (`schemaValidation: true`)

**Example**:
```
Input:  Validate("order_lookup", {"order_id": 123, "mode": "fast"})
Schema: {"type":"object", "properties":{"order_id":{"type":"string"},"mode":{"enum":["fast","full"]}}, "required":["order_id"]}
Error:  order_id must be string, got integer → 400 Bad Request
       {"error": "schema validation failed", "details": [{"field": "order_id", "reason": "expected string, got integer"}]}
```

### RULE-TR-003: Service Discovery and Heartbeat

**Trigger**: When the tool instance is registered / periodically

**constraint**:
- Tool instances are registered in the registry with heartbeats (interconnected with K8s Endpoints)
- Heartbeat TTL defaults to 30s (`discovery.heartbeatTTL`)
- Periodic health detection to eliminate unhealthy instances (exceeding TTL and no heartbeat → mark unhealthy)
-Support multi-instance load balancing

**Example**:
```
Register: POST /v1/tools/{name}/instances  {"endpoint":"http://10.0.1.5:8080", "weight":50}
Heartbeat: PUT /v1/tools/{name}/instances/{id}/heartbeat  Every 15s
Expire:   Redis TTL 30s Not refreshed → automatic eviction
          unhealthy Example from Resolve Exclude from return list
```

### RULE-TR-004: Capability package version and reference

**Trigger**: Register/Reference Skill, Rule, Spec

**constraint**:
- **Skill**: Press `name+version` for immutable storage; AgentSpec refers to `skill_ref` to get the latest compatible version (semantic, such as `^1.2.0`)
- **Rule**: stores OPA/Rego text; provides `Evaluate(input)` sandbox execution
- **Spec**: Stores AgentSpec template (§4.3.5) for low-code canvas/build path reference convergence

**Example**:
```
Skill Store:   skill_cache / code_review / v1.0.0, v1.1.0 (immutable)
AgentSpec Ref: skill_ref: "code_review@^1.0.0" → parsed as v1.1.0（Highest compatible version）

Rule Store:   rule_data_privacy / policy.rego (OPA)
Evaluate:     Evaluate({"data_field": "email", "output": "user@example.com"})
             → {allowed: false, reason: "PII in output"}
```

---

## Concurrency and Performance Rules (§9)

### RULE-TR-005: Hot path frame selection

**Trigger**: Code initialization phase

**constraint**:
- Management/Registration API: Gin
- `Resolve` is a hot path (Agent must pass through each tool call) and can go to Hertz/go-zero
- Do not allow hot paths to use synchronous blocking operations

### RULE-TR-006: Registry Caching Policy

**Trigger**: Registry read and write operations

**constraint**:
- Scenario of reading more and writing less: hot copy of registry stored in Redis
- Local `sync.RWMutex` protected `map` as first level cache
- Double writing fails when registering/unregistering (write PG first, then delete Redis + local cache)
- `Resolve` takes local cache hit, p99 ≤ 5ms

**Example**:
```
Read path:  Resolve("order_lookup", "tenant-A")
           → localCache.Get("tenant-A:order_lookup")  (RWMutex RLock)
           → miss → Redis GET
           → miss → PG SELECT
           → backfill Redis + localCache

Write path: Register(tool)
           → PG INSERT
           → Redis DEL
           → localCache.Invalidate
```

### RULE-TR-007: Goroutine model

**Trigger**: Every time a request arrives

**constraint**:
- One goroutine per request
- Tool proxy invocation (`invoke`) is controlled by context timeout
- Measurement `chan` + background worker asynchronously logs into the library without blocking the main path
- It is forbidden to write metric/audit in the middle of the main goroutine

### RULE-TR-008: Back pressure protection

**Trigger**: Slow response of downstream tools/increased concurrency

**constraint**:
- Tool instance concurrency upper limit uses semaphore
- If the downstream tool responds slowly, it will time out and return without piling up goroutines.
- After timeout, it will return `504 Gateway Timeout` directly without waiting for tool response.

**Example**:
```
Config:  toolInvokeTimeout=30s
Scenario: Tool example P99 delay from 200ms rise to 45s
Action:   New request waiting 30s → time out → 504 + "tool invocation timeout"
         No accumulation goroutine — context.Cancel clean up
```

### RULE-TR-009: Horizontal expansion

**Trigger**: Deploy configuration

**constraint**:
- No local state except rebuildable cache
- Horizontally scalable (multi-copy Deployment)
- Ensure consistency through PG authority + Redis/local cache invalidation under high concurrent registration/de-registration

---

## Safety Rules (§12)

### RULE-TR-010: Tool-level RBAC

**Trigger**: `Resolve` / `invoke` call

**constraint**:
- Declare the `rbac` role list when registering the tool (such as `["agent", "admin"]`)
- The caller's role is not in the list and returns `403 Forbidden`
- Tool-level API Key / OAuth2 support (Keycloak access is recommended for multi-user scenarios)
- Audit records include caller role + tool name + action

**Example**:
```
Tool Def:    {name: "db_query", rbac: ["admin"], auth: {type: "apikey"}}
Caller:      role=agent, tenant=T1
Check:       "agent" ∉ ["admin"] → 403 Forbidden
Audit:       {caller: "agent-42", tool: "db_query", action: "resolve", status: "denied"}
```

### RULE-TR-011: Tool call audit

**Trigger**: Every time a tool call is completed

**constraint**:
- Full audit (core): Each Resolve/Invoke is recorded to `audit_log`
- Audit fields: tenant_id, caller_id, tool_name, action, status, timestamp, duration_ms
- Asynchronous writing, does not block the tool from calling the main path
- Immutable: INSERT only

### RULE-TR-012: Schema injection protection

**Trigger**: Tool input parameter verification

**constraint**:
- All input parameters must be verified by JSON Schema (it is recommended to turn on `schemaValidation: true`)
- Illegal input is intercepted at the border of the registration center and is not passed to the tool implementation.
- Prevent SQL injection/command injection via tool parameter link
- Audit of verification failure records + return clear error information

### RULE-TR-013: Tool call metering

**Trigger**: Every time tool invoke completes

**constraint**:
- Record: number of calls, delay (ms), success rate
- Write to `tool_metrics` table (Prometheus synchronous collection)
- Multi-user scenarios need to be associated with tenant bills (to be aligned with ai-billing-service)

### RULE-TR-014: Capability package sandbox execution

**Trigger**: Rule's `Evaluate(input)` call

**constraint**:
- OPA/Rego rules are executed within the sandbox
- Disable rule access to host file system/network
- Execution timeout limit (e.g. 100ms)
- Exception rules do not affect the main service of the registration center

---

## Observability rules

- OTel traces + auditing is enabled by default (core)
- Prometheus indicators: number of registrations, Resolve QPS, number of calls, latency (p50/p95/p99), Schema verification failure rate
- Monitor hot copy consistency under high concurrent registration/de-registration (inconsistency triggers alarm)

---

## Traceability matrix

| Rules | Source Document DESIGN.md |
| --- | --- |
| RULE-TR-001~004 | §5 Key Algorithm |
| RULE-TR-005~009 | §9 Concurrency and Performance |
| RULE-TR-010~014 | §12 Observability/Security |

> **Change Log**: v0.1 | 2026-07-17 | First draft (extracted from DESIGN.md §5/§9/§12)
