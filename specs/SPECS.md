# ai-tool-registry · Specifications

> **规格层** — API/CLI 接口面、数据模型、部署配置
> **源文档**: design/DESIGN.md §7 / §8 / §11
> **平台版本**: v1.4.0

---

## 7. API / CLI 接口面

### 7.1 HTTP API

| 方法 | 路径 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| POST | `/v1/tools` | 注册工具 | Keycloak JWT |
| DELETE | `/v1/tools/{name}` | 反注册工具 | Keycloak JWT |
| GET | `/v1/tools` | 列出工具（按租户/标签过滤） | Keycloak JWT |
| POST | `/v1/tools/{name}/resolve` | 运行时解析（Agent 调用前） | Keycloak JWT |
| POST | `/v1/tools/{name}/invoke` | 代理调用工具（可选） | Keycloak JWT |
| POST | `/v1/skills` | 注册技能包 | Keycloak JWT |
| GET | `/v1/skills` | 列出技能包 | Keycloak JWT |
| POST | `/v1/rules` | 注册规则包 | Keycloak JWT |
| GET | `/v1/rules` | 列出规则包 | Keycloak JWT |
| POST | `/v1/specs` | 注册规范 | Keycloak JWT |
| GET | `/v1/specs` | 列出规范 | Keycloak JWT |
| GET | `/healthz` | 存活/就绪探针 | 无 |
| GET | `/metrics` | Prometheus 指标 | 内网 |

### 7.2 API 请求/响应模型

#### 注册工具

**请求体 (JSON)**:
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

#### Resolve 响应体

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

`aictl registry push ./tools/order_lookup.yaml` 由 `ai-cli` 转发；本仓亦可接受 `--config` 启动批量注册。

### 7.4 查询参数

| 参数 | 类型 | 用于 | 说明 |
| --- | --- | --- | --- |
| `tenant_id` | string | GET /v1/tools | 按租户过滤 |
| `kind` | string | GET /v1/tools | 按工具类型过滤（db/api/file/code/search/business） |
| `tags` | []string | GET /v1/tools | 按标签过滤 |
| `version` | string | GET /v1/skills | 按技能版本过滤 |

---

## 8. 数据模型

### 8.1 持久化存储

| 存储 | 角色 | 数据内容 |
| --- | --- | --- |
| PostgreSQL（core） | 权威存储 | `tools`（工具定义）、`tool_instances`（服务发现）、`skills`/`rules`/`specs`（能力包）、`tool_metrics`（计量）、`audit_log` |
| Redis（core） | 热数据 | 注册表热副本、计量滑动窗口、限流 |

### 8.2 核心表: `tools`

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

**列说明**:

| 列 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| name | TEXT | PK | 工具名称，如 `order_lookup` |
| tenant_id | TEXT | PK | 租户标识 |
| version | TEXT | PK | 语义化版本，如 `1.0.0` |
| kind | TEXT | | 工具类型: `db`/`api`/`file`/`code`/`search`/`business` |
| input_schema | JSONB | | JSON Schema 入参定义 |
| output_schema | JSONB | | JSON Schema 出参定义 |
| transport | TEXT | | MCP Transport: `stdio`/`sse`/`http`/`local` |
| endpoint | TEXT | | 默认实例地址 |
| auth | JSONB | | 鉴权配置: `{type: "apikey"/"oauth2"/"none"}` |
| rbac | JSONB | | 允许角色列表，如 `["agent", "admin"]` |
| tags | JSONB | | 语义检索标签 |

### 8.3 能力包表

| 表名 | 主键 | 说明 |
| --- | --- | --- |
| `skills` | `(tenant_id, name, version)` | 技能包注册 |
| `rules` | `(tenant_id, name)` | OPA/Rego 规则包 |
| `specs` | `(tenant_id, name)` | AgentSpec 模板 |

### 8.4 Redis 键设计

| Key Pattern | 用途 | TTL |
| --- | --- | --- |
| `tool:{tenant}:{name}` | 工具定义热副本 | 无（注册时写） |
| `tool:{tenant}:{name}:instances` | 服务发现实例列表 | 心跳 TTL 30s |
| `tool:metrics:{tenant}:{name}:count` | 调用计数滑动窗口 | 60s |
| `tool:metrics:{tenant}:{name}:latency` | 延迟滑动窗口 | 60s |

---

## 11. 配置与部署

### 11.1 部署形态

| 属性 | 值 |
| --- | --- |
| 必选性 | core |
| 命名空间 | `ai-system`（§9.2） |
| 部署方式 | Docker Compose（starter）/ K8s Deployment（standard） |
| 可选组件启停 | Skills/Rules/Specs 默认关（`skills.enabled: false` 等） |

### 11.2 K8s 资源配置

```yaml
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1
    memory: 1Gi
```

### 11.3 探针配置

| 探针 | 路径 | 说明 | initialDelaySeconds | periodSeconds |
| --- | --- | --- | --- | --- |
| 存活 | `GET /healthz` | 快速返回 200 | 5 | 10 |
| 就绪 | `GET /healthz` | 校验 PG + Redis 连接 | 5 | 10 |

### 11.4 滚动更新策略

```yaml
strategy:
  type: RollingUpdate
```

多副本 + 探针保活。

### 11.5 配置键完整列表

**文件位置**: `infrastructure/config/`

```yaml
toolRegistry:
  schemaValidation: true         # 入出参 JSON Schema 校验开关
  discovery:
    enabled: true                # 服务发现开关
    heartbeatTTL: 30s            # 实例心跳 TTL
  mcp:
    transports: [stdio, sse, http] # 支持的 MCP Transport
  metering:
    enabled: true                # 调用计量开关

auth:
  provider: keycloak             # 认证提供方

skills:
  enabled: false                 # Skills 管理开关（默认关）

rules:
  enabled: false                 # Rules 管理开关（默认关）

specs:
  enabled: false                 # Specs 管理开关（默认关）
```

**配置键说明**:

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `toolRegistry.schemaValidation` | bool | `true` | 启用 JSON Schema 入出参校验 |
| `toolRegistry.discovery.enabled` | bool | `true` | 启用服务发现 |
| `toolRegistry.discovery.heartbeatTTL` | duration | `30s` | 实例心跳 TTL |
| `toolRegistry.mcp.transports` | []string | `[stdio, sse, http]` | 支持的 MCP 传输协议 |
| `toolRegistry.metering.enabled` | bool | `true` | 启用调用计量 |
| `auth.provider` | string | `keycloak` | 认证提供方 |
| `skills.enabled` | bool | `false` | 启用 Skills 管理 |
| `rules.enabled` | bool | `false` | 启用 Rules 管理 |
| `specs.enabled` | bool | `false` | 启用 Specs 管理 |

### 11.6 阶段引入策略

| 阶段 | 组件 | 配置状态 |
| --- | --- | --- |
| 一~三（starter/standard） | 核心工具注册 | 工具注册/解析/校验/计量开启 |
| 三+（standard 点亮） | Skills/Rules/Specs | `skills/rules/specs.enabled=true` |
| 四（advanced/full） | 完整可选能力 | 全部开启 |

### 11.7 依赖组件

| 组件 | 类型 | 必选 | 说明 |
| --- | --- | --- | --- |
| PostgreSQL | 存储 | core | 工具定义/实例/能力包/审计 |
| Redis | 缓存 | core | 注册表热副本/计量/限流 |
| Keycloak | 认证 | core | 租户/用户 JWT |
| Qdrant（可选） | 向量库 | optional | 工具语义检索 |
| ai-sandbox-manager | 沙箱 | 间接 | 代码类 Tool 运行时（§10.6） |

---

## 追溯矩阵

| 章节 | 源文档 DESIGN.md 对应 |
| --- | --- |
| 7 API/CLI/配置接口面 | §7 |
| 8 数据模型与存储 | §8 |
| 11 配置与部署 | §11 |

> **变更记录**: v0.1 | 2026-07-17 | 初稿（从 DESIGN.md §7/§8/§11 提取）
