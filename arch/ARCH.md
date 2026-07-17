# ai-tool-registry · Architecture（架构总览）

> **摘自** `design/DESIGN.md` §1 定位与边界 · §2 职责清单 · §3 核心接口 · §6 适配器
> **语言·框架**: Go · Gin + Cobra + Wire（DDD 四层；热路径可上 Hertz/go-zero）
> **领域**: agent-infra（Agent 基础设施层 · 工具注册中心）
> **optional**: false（core · 核心；Agent 调用工具的核心）
> **平台版本**: v1.4.0

---

## §1 定位与边界（Scope）

### 1.1 一句话定位

`ai-tool-registry` 是 OpenStrata 的**工具注册中心**，承载架构 §4.3.2「工具注册中心」与 §7「Skills / Rules / Specs 管理平台」。它统一管理 Agent 可调用的**工具（Tool）**与**能力包（Skills / Rules / Specs）**的注册、Schema 校验、服务发现、鉴权与调用计量，是 Agent 经 `ToolRegistry` SPI 调用外部能力的唯一治理面。

### 1.2 解决的核心问题

把"分散在各处的工具（DB/API/文件/代码/搜索/业务）"与"平台级能力包（Skill/Rule/Spec）"收敛为**可声明、可校验、可发现、可治理**的注册表，让 Agent 运行时按 `tool_bindings`（§4.3.5）统一取用。

### 1.3 必选性与适用场景

- **必选**: core（推荐，§10.2「工具注册中心」）
- **最小场景**: Agent 需要调用任何工具时，本仓不可缺
- **可省略场景**: 纯 API 网关场景（无 Agent 调用工具需求）
- **标准部署**: 默认开，与 `ai-gateway-core` 配套

### 1.4 架构角色

| 维度 | 说明 |
| --- | --- |
| **层次** | DDD 四层：`domain/`（ToolRegistry Port + 能力包实体）· `application/`（注册/解析/校验用例）· `infrastructure/`（适配器/DI）· `interfaces/`（Gin handler） |
| **管理面框架** | Gin（工具注册/查询/管理 API） |
| **热路径框架** | Hertz/go-zero（可选，`Resolve` 为热路径，Agent 每次工具调用必经） |
| **DI 方案** | Wire（编译期依赖注入） |
| **协议支持** | MCP（Model Context Protocol）：stdio / SSE / HTTP 三种 Transport |

### 1.5 与其他 Go 组件的分工

| 组件 | 关系类型 | 说明 |
| --- | --- | --- | --- |
| `ai-gateway-core` | 请求链路串联 | 网关负责"模型调用"数据面；本仓负责"工具调用"治理面。Agent 调工具 → 本仓解析/鉴权/计量 → 工具实现（可能再经网关调模型）。 |
| `ai-sandbox-manager` | 间接依赖 | 代码类 Tool（`kind=code`）经本仓注册后，运行时由 `SandboxExecutor` 承载（§10.6 依赖规则 `Tool→SandboxExecutor`）。本仓不执行代码，只登记与路由。 |
| `ai-platform-api` | 控制面分层 | 控制面做租户/用户级授权汇总；本仓做工具级 RBAC 与实例发现。 |
| `ai-cli` | client/server | `aictl` 通过本仓 API 注册/查询工具与能力包（`aictl registry push`）。 |

### 1.6 边界约束

| 约束 | 说明 |
| --- | --- |
| **不执行工具** | 仅做注册、发现、路由、校验，工具执行在实际实例 |
| **不执行代码** | 代码类 Tool 运行时由 `ai-sandbox-manager` 承载 |
| **不做模型调用** | 模型调用是 `ai-gateway-core` 的职责 |
| **不做租户结算** | 计量数据上报 `ai-billing-service`，结算由其负责 |

---

## §2 职责清单

### 2.1 完整职责表

| # | 职责 | 必选/可选 | 触发条件 | 说明 |
| --- | --- | --- | --- | --- |
| R1 | **工具注册 / 反注册** | core | 管理员/CI 通过 API 注册 | YAML/JSON 声明工具 Schema，自动生成调用契约（§4.3.2） |
| R2 | **Schema 校验** | core（推荐） | 工具调用前/后 | 入参/出参 JSON Schema 校验，使用 gojsonschema（§4.3.2） |
| R3 | **服务发现** | optional | `discovery.enabled: true` 时 | 工具实例自动注册、心跳探活、负载均衡 |
| R4 | **工具鉴权（RBAC）** | optional（单用户可跳过） | `Resolve`/`invoke` 时 | 工具级 API Key / OAuth2（§4.3.2） |
| R5 | **调用计量** | optional | `metering.enabled: true` 时 | 调用次数、延迟、成功率统计（§4.3.2） |
| R6 | **Skills 管理** | optional（默认关） | `skills.enabled: true` 时 | 技能包注册/版本/绑定（§7.2） |
| R7 | **Rules 管理** | optional（默认关） | `rules.enabled: true` 时 | OPA/Rego 规则包注册与沙箱执行（§7.3） |
| R8 | **Specs 管理** | optional（默认关） | `specs.enabled: true` 时 | AgentSpec 模板/规范注册（§7.4，§4.3.5） |
| R9 | **MCP 协议接入** | 随工具注册 | 根据 `transport` 字段 | stdio / SSE / HTTP 三种 Transport 适配（§4.3.2） |

### 2.2 职责分级

| 级别 | 职责编号 | 数量 | 说明 |
| --- | --- | --- | --- |
| **core（不可关闭）** | R1, R2, R9 | 3 | 工具注册中心最小可行集 |
| **默认开（可配置关）** | R3, R4, R5 | 3 | 服务发现、鉴权、计量 |
| **默认关（optional）** | R6, R7, R8 | 3 | 能力包管理（进阶场景） |

### 2.3 启用策略

| 配置项 | 默认值 | 启用效果 | 关闭影响 |
| --- | --- | --- | --- |
| `schemaValidation` | `true` | 所有工具调用强制 Schema 校验 | 无校验，信任工具实现（不推荐） |
| `discovery.enabled` | `true` | 多实例负载均衡（weighted_random） | 仅用 `ToolDefinition.Endpoint` 单点 |
| `metering.enabled` | `true` | 调用统计（次数/延迟/成功率） | 无计量数据 |
| `skills.enabled` | `false` | 技能包注册与版本管理 | 无法引用技能包 |
| `rules.enabled` | `false` | OPA/Rego 规则执行 | 无规则能力 |
| `specs.enabled` | `false` | AgentSpec 模板管理 | 无 AgentSpec 模板 |

---

## §3 核心接口与抽象

### 3.1 设计原则

领域层（`domain/`）定义 `ToolRegistry` Port 与能力包实体。`ToolRegistry` 在 bom.yaml 的 15 个 SPI 端口中**无对应外部实例**（属平台自研 Port）。其"多实现"体现为**同一工具的多实例并存**（服务发现），而非外部组件替换（§10.3、§10.4）。

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
    Name         string            `json:"name"`          // 唯一，如 order_lookup
    Version      string            `json:"version"`
    Kind         string            `json:"kind"`          // db|api|file|code|search|business
    TenantID     string            `json:"tenant_id"`
    InputSchema  map[string]any    `json:"input_schema"`  // JSON Schema
    OutputSchema map[string]any    `json:"output_schema"`
    Transport    string            `json:"transport"`     // stdio|sse|http|local
    Endpoint     string            `json:"endpoint"`      // 实例地址（服务发现填充）
    Auth         ToolAuth          `json:"auth"`          // apikey|oauth2|none
    RBAC         []string          `json:"rbac"`          // 允许 role 列表
    CapabilityTags []string        `json:"tags"`          // 语义检索标签
}

type ResolvedTool struct {
    Def       ToolDefinition
    Instances []ToolInstance       // 服务发现得到的多实例（负载均衡）
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

### 3.3 能力包实体

```go
// ===== 能力包实体（§7）=====
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
    PolicyRego string `json:"policy_rego"` // OPA/Rego 完整策略文本
    Severity   string `json:"severity"`    // low|medium|high|critical
    TenantID   string `json:"tenant_id"`
    CreatedAt  time.Time `json:"created_at"`
}

type Spec struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    AgentSpecRef string `json:"agent_spec_ref"` // 引用 §4.3.5 AgentSpec
    TenantID     string `json:"tenant_id"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### 3.4 延迟预算

| 操作 | 预算 | 说明 |
| --- | --- | --- |
| `Resolve`（本地缓存命中） | ≤5ms (p99) | `sync.RWMutex` 保护本地 map 读取 |
| `Resolve`（Redis 回源） | ≤10ms (p99) | 本地缓存未命中时 |
| `Schema 校验` | ≤2ms | gojsonschema 单次校验 |
| 服务发现（Redis） | ≤3ms | 取健康实例列表 |
| 代理调用（`invoke`） | 依赖下游工具 | context 超时控制，默认 30s |

### 3.5 调用路径概要

```
Agent 运行时 → Resolve(name, tenant) → 租户上下文 + RBAC 校验
  → Schema 校验入参 → 服务发现取健康实例 → loadBalance 选实例
  → MCP Transport 调工具实现(stdio/SSE/HTTP) → 调用计量/出参校验 → 返回
```

> 能力包（Skill/Rule/Spec）注册路径为"声明→校验→入库→可被 AgentSpec 引用"，不走运行时调用链路（§7）。

---

## §6 适配器与 SPI 生态

### 6.1 SPI 端口矩阵

| SPI 端口 | 本仓角色 | 外部组件（bom.yaml） | 默认 ✅ / 备选 | Adapter |
| --- | --- | --- | --- | --- |
| `ToolRegistry` | 实现方 | 本仓自身（无外部 SPI 实例，属平台自研 Port） | — | MCP 协议接入（stdio/SSE/HTTP） |
| `VectorStore` (1.1.0) | 消费方（可选） | Qdrant（core）/ Milvus（optional） | ✅ / 备选 | 工具语义检索/标签向量化（可选增强） |
| `Cache` (1.0.0) | 消费方 | Redis（core）/ Valkey（optional） | ✅ / 备选 | 注册表热副本、计量计数 |
| `Auth` (1.0.0) | 消费方 | Keycloak（core） | ✅ | 租户/用户 JWT 身份校验 |
| `Sandbox` (1.0.0) | 间接依赖 | Kata/E2B（optional） | 备选 | 代码类 Tool 运行时经 `SandboxExecutor` 承载 |

### 6.2 MCP Transport Adapter

| Transport | 协议特性 | 适用场景 | Adapter 关键处理 |
| --- | --- | --- | --- |
| `stdio` | 标准输入/输出，子进程通信 | 本地 CLI 工具、Python/Node 脚本 | 进程启动管理 + stdin/stdout pipe + 进程生命周期 |
| `SSE` | Server-Sent Events 单向流 | 流式工具、实时推送 | HTTP 长连接管理 + 事件流解析 + 断线重连 |
| `HTTP` | REST / gRPC 标准调用 | 远程微服务工具 | 标准 HTTP client + JSON 序列化 + 连接池 |

### 6.3 防腐层（ACL）

MCP 三种 Transport 的差异在 Adapter 内收敛为统一 `ResolvedTool` 调用。调用方无需感知底层传输差异：

- `stdio` Adapter：子进程管理，自动重连
- `SSE` Adapter：长连接事件流 → 内部 result channel
- `HTTP` Adapter：标准 REST → 内部 response struct

### 6.4 ToolRegistry 多实现策略

`ToolRegistry` 在 bom.yaml 的 15 个 SPI 端口中**无对应外部实例**（属平台自研 Port）。其"多实现"体现为：

1. **同一工具名支持多版本**: `name+version` 不可变存储，AgentSpec 按语义化版本引用
2. **同一版本支持多实例**: 服务发现返回 `[]ToolInstance`，按 `Weight` 加权随机/轮询
3. **热副本机制**: Redis 主副本 + 本地 `sync.RWMutex` 保护 map，`Resolve` p99 ≤ 5ms

### 6.5 注册表缓存策略

| 缓存层 | 存储 | 读延迟 | 一致性 |
| --- | --- | --- | --- |
| L1（本地） | `sync.RWMutex` 保护 map | ≤1ms | 最终一致（双写失效） |
| L2（Redis） | Hash（JSON） | ≤3ms | 强一致（写穿透 PG） |
| 权威 | PostgreSQL | ≤10ms | 强一致 |

双写流程: 注册工具 → 写 PG（权威）→ 删 Redis 热副本 → 本地缓存失效。`Resolve` 优先读 L1，miss 回源 L2，再 miss 查 PG 并回填。

### 6.6 间接依赖：SandboxExecutor 绑定

代码类 Tool（`kind=code`）经本仓注册后，运行时由 `ai-sandbox-manager` 的 `SandboxExecutor` 承载：

```go
// TenantCode 中声明的依赖规则（§10.6）
// Tool(ai-tool-registry) → SandboxExecutor(ai-sandbox-manager)
```

- 注册时声明 `kind: code` 不强制 `ai-sandbox-manager` 在线（弱依赖）
- 但调用时若沙箱不可用，返回 `503 Sandbox Unavailable`
- `ai-provisioning-engine` 在装配时检测此依赖规则并提示

### 6.7 可选增强组件

| 组件 | 启用条件 | 功能 | 实现方式 |
| --- | --- | --- | --- |
| VectorStore（Qdrant/Milvus） | `vectorSearch.enabled: true` | 工具语义检索、标签向量化 | 工具标签 → Embedding → VectorStore 索引 |
| SandboxExecutor | 代码类 Tool 注册 + 调用时 | 代码沙箱执行 | 经 `ai-sandbox-manager` Sandbox SPI |

---

## 请求路径全景

### 工具运行时调用（热路径）

```
Agent 运行时 / ai-platform-api
  → POST /v1/tools/{name}/resolve (JWT)
    → ai-tool-registry 接入层 handler [Gin / Hertz]
      → 租户上下文解析（Keycloak JWT → tenant_id / role）
        → 工具级 RBAC 校验（ToolDefinition.RBAC vs caller role）
          → [无权] → 403 Forbidden + 审计
          → [通过] → Schema 校验入参（gojsonschema）
            → [非法] → 400 Bad Request + 错误明细
            → [合法] → 服务发现: 取健康实例（Redis 热副本）
              → 负载均衡选实例（weighted_random / round_robin）
                → 经 MCP Transport 调工具实现（stdio/SSE/HTTP）
                  → 调用计量: 次数/延迟（异步写入 tool_metrics）
                    → 出参 Schema 校验（告警不阻塞）
                      → 返回 ResolvedTool + 审计

能力包注册路径（管理面）:
声明 → JSON Schema 校验 → 入库 PG → 可被 AgentSpec 引用（§7）
```

---

> **关联文档**: 本仓 `design/DESIGN.md` · `skills/SKILLS.md` · `specs/SPECS.md`
> **架构引用**: §4.3.2（工具注册中心）· §7（Skills/Rules/Specs管理）· §4.3.5（AgentSpec 工具绑定）· §10.3（ToolRegistry SPI）· §10.6（Component Registry）· §15.6（DDD分层）· §16（BOM）
