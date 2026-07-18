# ai-tool-registry

> OpenStrata tool registry — the Agent tool registration center (DESIGN §1). It
> uniformly manages registration, JSON-Schema validation, service discovery,
> tool-level RBAC, call metering, and Skills/Rules/Specs capability packages.
> It is the only management surface through which the Agent calls external tools
> via the `ToolRegistry` SPI (DESIGN §3.2 / §10.3).

- **Language**: Go (stdlib-only, offline-verifiable `go build`/`go vet`/`go test`).
  Production hot path runs behind Higress with Keycloak JWT + PostgreSQL/Redis;
  the docs describe Gin/Hertz/go-zero + Wire (DESIGN §1.4) — implemented here as
  hand-wired stdlib `net/http` so the build has zero third-party dependencies.
- **domain**: agent-infra
- **OPTIONAL**: core
- **Module**: `github.com/open-strata-ai/ai-tool-registry`
- **Meta repository reference**: `openstrata-meta/repos.yaml` (tag `v1.0.0`) · BOM `openstrata-meta/bom.yaml`

## Repository structure (DDD four layers, §15.5)

```
ai-tool-registry/
├── cmd/                       # Bootstrap: hand-wired dependency graph (Wire stand-in)
├── domain/                    # ToolRegistry/Store/Discovery/Schema/Transport ports + types + errors
├── application/
│   ├── registry/              # Core use case: register/resolve/list/deregister/validate/invoke
│   ├── schema/                # Stdlib JSON-Schema structural validator (R2 / §5.2)
│   ├── rbac/                  # Tool-level role check (R4 / §5.1)
│   ├── discovery/             # Weighted-random / round-robin instance picker (R3 / §5.3)
│   └── metering/              # Async, non-blocking call metering + aggregates (R5 / §12)
├── infrastructure/
│   ├── config/                # Typed config mirroring DESIGN §11.5 (JSON overlay)
│   ├── auth/                  # AuthPort stub (Keycloak JWT in production)
│   ├── store/                 # In-memory domain.Store (PostgreSQL + Redis in prod)
│   ├── discovery/             # In-memory domain.Discovery (K8s Endpoints + Redis in prod)
│   ├── mcp/                   # MCP transport anti-corrosion layer (http/stdio/sse/local)
│   └── metering/              # Log/Discard sinks for the metering Recorder
├── interfaces/http/           # net/http handler + tests (SPECS §7)
├── Dockerfile / Makefile      # Deployment + build/test/run targets
└── docs/                      # ARCH.md, DESIGN.md, SKILLS.md, SPECS.md, adr/
```

## HTTP API (SPECS §7)

| Method | Path | Description |
| --- | --- | --- |
| POST | `/v1/tools` | Register a tool |
| DELETE | `/v1/tools/{name}` | Deregister a tool (all versions) |
| GET | `/v1/tools` | List tools (filter by `tenant_id`/`kind`/`tags`/`limit`) |
| POST | `/v1/tools/{name}/resolve` | Runtime resolve (RBAC + schema + discovery) |
| POST | `/v1/tools/{name}/invoke` | Proxy invoke over the tool's MCP transport |
| POST/GET | `/v1/skills` `/v1/rules` `/v1/specs` | Capability packages (off by default, §11.5) |
| GET | `/healthz` | Liveness/readiness |
| GET | `/metrics` | Prometheus-style call metrics |

## Local development

```bash
make build   # go build ./...
make test    # go test ./...
make lint    # go vet ./...
make run     # go run ./cmd  (listens on 0.0.0.0:8080)
```

Offline auth trusts the `X-Tenant-Id` header (and `Bearer tenant:role`). The
registry never executes tools — it routes to discovered instances; `code`/`stdio`
runtimes are hosted by `ai-sandbox-manager` (DESIGN §1.6 / §6.7).

> Evolutionary AI coding: `docs/` is the source of truth shared by AI assistants
> and contributors; new decisions are recorded as ADRs in `docs/adr/`.
