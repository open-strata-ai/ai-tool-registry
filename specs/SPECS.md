# ai-tool-registry · Specifications

> **Specification layer** — API/CLI interface, data model, deployment configuration
> **Source document**: design/DESIGN.md §7 / ​​§8 / §11
> **Platform version**: v1.4.0

---

## 7. API / CLI interface

### 7.1 HTTP API

| Method | Path | Description | Authentication |
| --- | --- | --- | --- |
| POST | `/v1/tools` | Registration Tools | Keycloak JWT |
| DELETE | `/v1/tools/{name}` | Anti-registration tool | Keycloak JWT |
| GET | `/v1/tools` | List tools (filtered by tenant/tag) | Keycloak JWT |
| POST | `/v1/tools/{name}/resolve` | Runtime resolution (before Agent call) | Keycloak JWT |
| POST | `/v1/tools/{name}/invoke` | Agent invocation tool (optional) | Keycloak JWT |
| POST | `/v1/skills` | Register skills package | Keycloak JWT |
| GET | `/v1/skills` | List skill packs | Keycloak JWT |
| POST | `/v1/rules` | Registration rules package | Keycloak JWT |
| GET | `/v1/rules` | List rule packages | Keycloak JWT |
| POST | `/v1/specs` | Registration specifications | Keycloak JWT |
| GET | `/v1/specs` | List specifications | Keycloak JWT |
| GET | `/healthz` | Liveness/readiness probe | None |
| GET | `/metrics` | Prometheus metrics | Intranet |

### 7.2 API request/response model

#### Registration tool

**Request body (JSON)**:
```json
{
  "name": "order_lookup",
  "version": "1.0.0",
  "kind": "api",
  "tenant_id": "tenant-A",
  "input_schema": {
    "type": "object",
    "properties": {
      "order_id": {"type": "string"}
    },
    "required": ["order_id"]
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "status": {"type": "string"},
      "amount": {"type": "number"}
    }
  },
  "transport": "http",
  "endpoint": "http://order-svc:8080/lookup",
  "auth": {"type": "apikey"},
  "rbac": ["agent"],
  "tags": ["order", "erp"]
}
```

#### Resolve response body

```json
{
  "def": {
    "name": "order_lookup",
    "version": "1.0.0",
    "kind": "api",
    "transport": "http"
  },
  "instances": [
    {"endpoint": "http://10.0.1.5:8080", "healthy": true, "weight": 60},
    {"endpoint": "http://10.0.2.8:8080", "healthy": true, "weight": 40}
  ]
}
```

### 7.3 CLI

`aictl registry push ./tools/order_lookup.yaml` is forwarded by `ai-cli`; this repository can also accept `--config` to start batch registration.

### 7.4 Query parameters

| Parameters | Type | Used | Description |
| --- | --- | --- | --- |
| `tenant_id` | string | GET /v1/tools | Filter by tenant |
| `kind` | string | GET /v1/tools | Filter by tool type (db/api/file/code/search/business) |
| `tags` | []string | GET /v1/tools | Filter by tags |
| `version` | string | GET /v1/skills | Filter by skill version |

---

## 8. Data model

### 8.1 Persistent storage

| Storage | Role | Data Content |
| --- | --- | --- |
| PostgreSQL (core) | Authoritative storage | `tools` (tool definition), `tool_instances` (service discovery), `skills`/`rules`/`specs` (capability package), `tool_metrics` (measurement), `audit_log` |
| Redis (core) | Hot data | Registry hot copy, metering sliding window, rate limiting |

### 8.2 Core table: `tools`

```sql
CREATE TABLE tools (
  name          TEXT NOT NULL,
  tenant_id     TEXT NOT NULL,
  version       TEXT NOT NULL,
  kind          TEXT,
  input_schema  JSONB,
  output_schema JSONB,
  transport     TEXT,
  endpoint      TEXT,
  auth          JSONB,
  rbac          JSONB,
  tags          JSONB,
  PRIMARY KEY (tenant_id, name, version)
);
```

**Column Description**:

| Column | Type | Constraint | Description |
| --- | --- | --- | --- |
| name | TEXT | PK | Tool name, such as `order_lookup` |
| tenant_id | TEXT | PK | Tenant ID |
| version | TEXT | PK | Semantic version, such as `1.0.0` |
| kind | TEXT | | Tool type: `db`/`api`/`file`/`code`/`search`/`business` |
| input_schema | JSONB | | JSON Schema input parameter definition |
| output_schema | JSONB | | JSON Schema output parameter definition |
| transport | TEXT | | MCP Transport: `stdio`/`sse`/`http`/`local` |
| endpoint | TEXT | | Default instance address |
| auth | JSONB | | Authentication configuration: `{type: "apikey"/"oauth2"/"none"}` |
| rbac | JSONB | | List of allowed roles, such as `["agent", "admin"]` |
| tags | JSONB | | Semantic search tags |

### 8.3 Capability package table

| Table name | Primary key | Description |
| --- | --- | --- |
| `skills` | `(tenant_id, name, version)` | Skill pack registration |
| `rules` | `(tenant_id, name)` | OPA/Rego rule package |
| `specs` | `(tenant_id, name)` | AgentSpec template |

### 8.4 Redis key design

| Key Pattern | Purpose | TTL |
| --- | --- | --- |
| `tool:{tenant}:{name}` | Hot copy of tool definition | None (written during registration) |
| `tool:{tenant}:{name}:instances` | Service discovery instance list | Heartbeat TTL 30s |
| `tool:metrics:{tenant}:{name}:count` | Call count sliding window | 60s |
| `tool:metrics:{tenant}:{name}:latency` | Latency sliding window | 60s |

---

## 11. Configuration and deployment

### 11.1 Deployment form

| Properties | Values ​​|
| --- | --- |
| Required | core |
| namespace | `ai-system` (§9.2) |
| Deployment method | Docker Compose (starter)/K8s Deployment (standard) |
| Optional component start and stop | Skills/Rules/Specs is off by default (`skills.enabled: false`, etc.) |

### 11.2 K8s resource configuration

```yaml
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1
    memory: 1Gi
```

### 11.3 Probe configuration

| probe | path | description | initialDelaySeconds | periodSeconds |
| --- | --- | --- | --- | --- |
| Alive | `GET /healthz` | Quick return 200 | 5 | 10 |
| Ready | `GET /healthz` | Verify PG + Redis connection | 5 | 10 |

### 11.4 Rolling update strategy

```yaml
strategy:
  type: RollingUpdate
```

Multiple copies + probe keep alive.

### 11.5 Complete list of configuration keys

**File location**: `infrastructure/config/`

```yaml
toolRegistry:
  schemaValidation: true         #Input and output parameters JSON Schema verification switch
  discovery:
    enabled: true                #Service discovery switch
    heartbeatTTL: 30s            #Instance heartbeat TTL
  mcp:
    transports: [stdio, sse, http] #Supported MCP Transport
  metering:
    enabled: true                #Call metering switch

auth:
  provider: keycloak             #Certification provider

skills:
  enabled: false                 #Skills management switch (default off)

rules:
  enabled: false                 #Rules management switch (off by default)

specs:
  enabled: false                 #Specs management switch (default off)
```

**Configuration key description**:

| key | type | default value | description |
| --- | --- | --- | --- |
| `toolRegistry.schemaValidation` | bool | `true` | Enable JSON Schema input and output parameter verification |
| `toolRegistry.discovery.enabled` | bool | `true` | Enable service discovery |
| `toolRegistry.discovery.heartbeatTTL` | duration | `30s` | Instance heartbeat TTL |
| `toolRegistry.mcp.transports` | []string | `[stdio, sse, http]` | Supported MCP transport protocols |
| `toolRegistry.metering.enabled` | bool | `true` | Enable call metering |
| `auth.provider` | string | `keycloak` | Authentication provider |
| `skills.enabled` | bool | `false` | Enable Skills management |
| `rules.enabled` | bool | `false` | Enable Rules management |
| `specs.enabled` | bool | `false` | Enable Specs management |

### 11.6 Stage introduction strategy

| Stages | Components | Configuration Status |
| --- | --- | --- |
| One to three (starter/standard) | Core tool registration | Tool registration/parsing/verification/measurement start |
| Three+ (standard lit) | Skills/Rules/Specs | `skills/rules/specs.enabled=true` |
| Four (advanced/full) | Complete optional capabilities | All enabled |

### 11.7 Dependent components

| Component | Type | Required | Description |
| --- | --- | --- | --- |
| PostgreSQL | storage | core | tool definition/instance/capability package/audit |
| Redis | cache | core | registry hot copy/metering/rate limiting |
| Keycloak | authentication | core | tenant/user JWT |
| Qdrant (optional) | Vector library | optional | Tool semantic retrieval |
| ai-sandbox-manager | Sandbox | Indirect | Code Class Tool Runtime (§10.6) |

---

## Traceability matrix

| Chapter | Source document DESIGN.md corresponding |
| --- | --- |
| 7 API/CLI/Configuration Interface | §7 |
| 8 Data Model and Storage | §8 |
| 11 Configuration and Deployment | §11 |

> **Change Record**: v0.1 | 2026-07-17 | First draft (extracted from DESIGN.md §7/§8/§11)
