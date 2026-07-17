# ai-tool-registry · Skills & Rules

> **技能规则层** — 关键算法、并发/性能模型、安全策略的可执行规则
> **源文档**: design/DESIGN.md §5 / §9 / §12
> **平台版本**: v1.4.0

---

## 算法规则（§5）

### RULE-TR-001: 工具解析与路由

**触发**: 调用 `Resolve(name, tenantID)`

**约束**:
1. 按 `tenant_id + name` 定位 `ToolDefinition`（主键: `tenant_id, name, version`）
2. 据 `RBAC` 校验 caller role（无匹配 role 返回 403）
3. 从 `service_discovery` 取健康实例
4. 按实例 `Weight` 加权随机/轮询选实例
5. 返回 `ResolvedTool{Def, Instances}`

**示例**:
```
Input:  Resolve("order_lookup", "tenant-A")
Step1:  PG SELECT WHERE tenant_id='tenant-A' AND name='order_lookup' ORDER BY version DESC LIMIT 1
Step2:  RBAC check: caller role=agent → ["agent"] ∈ ToolDefinition.RBAC → pass
Step3:  Redis SMEMBERS "tool:order_lookup:tenant-A:instances" → [inst1, inst2]
Step4:  Weight=60(inst1), 40(inst2) → weighted random → inst1
Output: ResolvedTool{Def: {...}, Instances: [{Endpoint:"http://inst1:8080", Healthy:true, Weight:60}]}
```

### RULE-TR-002: Schema 校验

**触发**: 工具调用前（`Validate` 或 `invoke` 路径）

**约束**:
- 入参/出参用 JSON Schema（gojsonschema）校验
- 非法入参在注册中心边界即拦截，返回 400 + 错误明细
- 校验失败不传递到工具实现（防止污染）
- 建议开启（`schemaValidation: true`）

**示例**:
```
Input:  Validate("order_lookup", {"order_id": 123, "mode": "fast"})
Schema: {"type":"object", "properties":{"order_id":{"type":"string"},"mode":{"enum":["fast","full"]}}, "required":["order_id"]}
Error:  order_id must be string, got integer → 400 Bad Request
       {"error": "schema validation failed", "details": [{"field": "order_id", "reason": "expected string, got integer"}]}
```

### RULE-TR-003: 服务发现与心跳

**触发**: 工具实例注册时 / 周期性

**约束**:
- 工具实例以心跳注册到注册表（对接 K8s Endpoints）
- 心跳 TTL 默认 30s（`discovery.heartbeatTTL`）
- 周期性健康探测剔除不健康实例（超 TTL 无心跳 → 标记 unhealthy）
- 支持多实例负载均衡

**示例**:
```
Register: POST /v1/tools/{name}/instances  {"endpoint":"http://10.0.1.5:8080", "weight":50}
Heartbeat: PUT /v1/tools/{name}/instances/{id}/heartbeat  每 15s
Expire:   Redis TTL 30s 未刷新 → 自动驱逐
          unhealthy 实例从 Resolve 返回列表中排除
```

### RULE-TR-004: 能力包版本与引用

**触发**: 注册/引用 Skill, Rule, Spec

**约束**:
- **Skill**: 按 `name+version` 不可变存储；AgentSpec 引用 `skill_ref` 取最新兼容版本（语义化，如 `^1.2.0`）
- **Rule**: 存储 OPA/Rego 文本；提供 `Evaluate(input)` 沙箱执行
- **Spec**: 存储 AgentSpec 模板（§4.3.5），供低代码画布/构建路径引用收敛

**示例**:
```
Skill Store:   skill_cache / code_review / v1.0.0, v1.1.0 (不可变)
AgentSpec Ref: skill_ref: "code_review@^1.0.0" → 解析为 v1.1.0（最高兼容版本）

Rule Store:   rule_data_privacy / policy.rego (OPA)
Evaluate:     Evaluate({"data_field": "email", "output": "user@example.com"})
             → {allowed: false, reason: "PII in output"}
```

---

## 并发与性能规则（§9）

### RULE-TR-005: 热路径框架选择

**触发**: 代码初始化阶段

**约束**:
- 管理/注册 API：Gin
- `Resolve` 为热路径（Agent 每次工具调用必经），可上 Hertz/go-zero
- 不允许热路径使用同步阻塞操作

### RULE-TR-006: 注册表缓存策略

**触发**: 注册表读写操作

**约束**:
- 读多写少场景：注册表热副本存 Redis
- 本地 `sync.RWMutex` 保护的 `map` 作为一级缓存
- 注册/反注册时双写失效（先写 PG，再删 Redis + 本地缓存）
- `Resolve` 走本地缓存命中，p99 ≤ 5ms

**示例**:
```
Read path:  Resolve("order_lookup", "tenant-A")
           → localCache.Get("tenant-A:order_lookup")  (RWMutex RLock)
           → miss → Redis GET
           → miss → PG SELECT
           → 回填 Redis + localCache

Write path: Register(tool)
           → PG INSERT
           → Redis DEL
           → localCache.Invalidate
```

### RULE-TR-007: Goroutine 模型

**触发**: 每次请求到达

**约束**:
- 每请求一个 goroutine
- 工具代理调用（`invoke`）用 context 超时控制
- 计量经 `chan` + 后台 worker 异步落库，不阻塞主路径
- 禁止在主 goroutine 中间步写 metric/audit

### RULE-TR-008: 背压保护

**触发**: 下游工具慢响应 / 并发升高

**约束**:
- 工具实例并发上限用信号量
- 下游工具慢响应时超时即返回，不堆积 goroutine
- 超时后不等待工具响应，直接返回 `504 Gateway Timeout`

**示例**:
```
Config:  toolInvokeTimeout=30s
Scenario: 工具实例 P99 延迟从 200ms 升到 45s
Action:   新请求等待 30s → 超时 → 504 + "tool invocation timeout"
         不堆积 goroutine — context.Cancel 清理
```

### RULE-TR-009: 水平扩展

**触发**: 部署配置

**约束**:
- 除可重建缓存外无本地状态
- 可水平扩缩（多副本 Deployment）
- 高并发注册/反注册下通过 PG 权威 + Redis/本地缓存失效保证一致性

---

## 安全规则（§12）

### RULE-TR-010: 工具级 RBAC

**触发**: `Resolve` / `invoke` 调用

**约束**:
- 工具注册时声明 `rbac` 角色列表（如 `["agent", "admin"]`）
- 调用方 role 不在列表内返回 `403 Forbidden`
- 工具级 API Key / OAuth2 支持（多用户场景推荐 Keycloak 接入）
- 审计记录包含 caller role + tool name + action

**示例**:
```
Tool Def:    {name: "db_query", rbac: ["admin"], auth: {type: "apikey"}}
Caller:      role=agent, tenant=T1
Check:       "agent" ∉ ["admin"] → 403 Forbidden
Audit:       {caller: "agent-42", tool: "db_query", action: "resolve", status: "denied"}
```

### RULE-TR-011: 工具调用审计

**触发**: 每次工具调用完成

**约束**:
- 全量审计（core）：每次 Resolve/Invoke 记录到 `audit_log`
- 审计字段: tenant_id, caller_id, tool_name, action, status, timestamp, duration_ms
- 异步写入，不阻塞工具调用主路径
- 不可变：仅 INSERT

### RULE-TR-012: Schema 注入防护

**触发**: 工具输入参数校验

**约束**:
- 所有入参必经 JSON Schema 校验（建议开启 `schemaValidation: true`）
- 非法输入在注册中心边界拦截，不传递到工具实现
- 防止 SQL 注入 / 命令注入通过工具参数链路
- 校验失败记录审计 + 返回明确错误信息

### RULE-TR-013: 工具调用计量

**触发**: 每次工具 invoke 完成

**约束**:
- 记录: 调用次数、延迟（ms）、成功率
- 写入 `tool_metrics` 表（Prometheus 同步采集）
- 多用户场景需关联租户账单（待与 ai-billing-service 对齐）

### RULE-TR-014: 能力包沙箱执行

**触发**: Rule 的 `Evaluate(input)` 调用

**约束**:
- OPA/Rego 规则在沙箱内执行
- 禁止规则访问宿主文件系统/网络
- 执行超时限制（如 100ms）
- 异常规则不影响注册中心主服务

---

## 可观测性规则

- OTel traces + 审计默认开（core）
- Prometheus 指标: 注册数, Resolve QPS, 调用次数, 延迟(p50/p95/p99), Schema 校验失败率
- 高并发注册/反注册下监控热副本一致性（不一致触发告警）

---

## 追溯矩阵

| 规则 | 源文档 DESIGN.md |
| --- | --- |
| RULE-TR-001~004 | §5 关键算法 |
| RULE-TR-005~009 | §9 并发与性能 |
| RULE-TR-010~014 | §12 可观测性/安全 |

> **变更记录**: v0.1 | 2026-07-17 | 初稿（从 DESIGN.md §5/§9/§12 提取）
