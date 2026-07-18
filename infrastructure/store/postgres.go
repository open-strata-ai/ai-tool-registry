package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	_ "github.com/lib/pq"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Postgres is a PostgreSQL-backed domain.Store.
type Postgres struct {
	db *sql.DB
}

func NewPostgres(dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres store: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres store: ping: %w", err)
	}
	if _, err := db.Exec(migrateToolRegistry); err != nil {
		return nil, fmt.Errorf("postgres store: migrate: %w", err)
	}
	return &Postgres{db: db}, nil
}

const migrateToolRegistry = `
CREATE TABLE IF NOT EXISTS tools (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL DEFAULT '',
    version       TEXT NOT NULL DEFAULT '',
    input_schema  JSONB,
    output_schema JSONB,
    transport     TEXT NOT NULL DEFAULT '',
    endpoint      TEXT NOT NULL DEFAULT '',
    auth          JSONB NOT NULL DEFAULT '{}',
    rbac          TEXT[] NOT NULL DEFAULT '{}',
    tags          TEXT[] NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS skills (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    version    TEXT NOT NULL DEFAULT '',
    manifest   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS rules (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    policy_rego TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS specs (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL DEFAULT '',
    agent_spec_ref TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tools_tenant ON tools(tenant_id);
CREATE INDEX IF NOT EXISTS idx_skills_tenant ON skills(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rules_tenant  ON rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_specs_tenant  ON specs(tenant_id);`

func toolID(tenant, name, version string) string { return tenant + "/" + name + "/" + version }

func (p *Postgres) SaveTool(def domain.ToolDefinition) error {
	id := toolID(def.TenantID, def.Name, def.Version)
	isJSON, _ := json.Marshal(def.InputSchema)
	osJSON, _ := json.Marshal(def.OutputSchema)
	authJSON, _ := json.Marshal(def.Auth)
	tags := "{}"
	if len(def.CapabilityTags) > 0 {
		tags = "{" + strings.Join(def.CapabilityTags, ",") + "}"
	}
	rbac := "{}"
	if len(def.RBAC) > 0 {
		rbac = "{" + strings.Join(def.RBAC, ",") + "}"
	}
	_, err := p.db.Exec(`
		INSERT INTO tools (id,tenant_id,name,kind,version,input_schema,output_schema,transport,endpoint,auth,rbac,tags)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,$9,$10::jsonb,$11::text[],$12::text[])
		ON CONFLICT (id) DO UPDATE SET
			kind=EXCLUDED.kind, version=EXCLUDED.version, input_schema=EXCLUDED.input_schema,
			output_schema=EXCLUDED.output_schema, transport=EXCLUDED.transport, endpoint=EXCLUDED.endpoint,
			auth=EXCLUDED.auth, rbac=EXCLUDED.rbac, tags=EXCLUDED.tags, updated_at=NOW()`,
		id, def.TenantID, def.Name, def.Kind, def.Version, string(isJSON), string(osJSON),
		def.Transport, def.Endpoint, string(authJSON), rbac, tags,
	)
	return err
}

func rowToTool(scanner interface{ Scan(...interface{}) error }) (domain.ToolDefinition, error) {
	var def domain.ToolDefinition
	var isJSON, osJSON, authJSON, tagsStr, rbacStr string
	err := scanner.Scan(&def.Name, &def.Version, &def.Kind, &def.TenantID,
		&isJSON, &osJSON, &def.Transport, &def.Endpoint, &authJSON, &rbacStr, &tagsStr)
	if err != nil {
		return def, err
	}
	json.Unmarshal([]byte(isJSON), &def.InputSchema)
	json.Unmarshal([]byte(osJSON), &def.OutputSchema)
	json.Unmarshal([]byte(authJSON), &def.Auth)
	def.CapabilityTags = splitCSV(tagsStr)
	def.RBAC = splitCSV(rbacStr)
	return def, nil
}

func splitCSV(s string) []string {
	if s == "" || s == "{}" {
		return nil
	}
	s = strings.Trim(s, "{}")
	out := strings.Split(s, ",")
	if len(out) == 1 && out[0] == "" {
		return nil
	}
	return out
}

func (p *Postgres) GetTool(tenant, name, version string) (domain.ToolDefinition, bool) {
	def, err := rowToTool(p.db.QueryRow(
		`SELECT name,version,kind,tenant_id,input_schema,output_schema,transport,endpoint,auth,rbac,tags FROM tools WHERE tenant_id=$1 AND name=$2 AND version=$3`,
		tenant, name, version))
	if err != nil {
		return domain.ToolDefinition{}, false
	}
	return def, true
}

func (p *Postgres) GetLatestTool(tenant, name string) (domain.ToolDefinition, bool) {
	def, err := rowToTool(p.db.QueryRow(
		`SELECT name,version,kind,tenant_id,input_schema,output_schema,transport,endpoint,auth,rbac,tags FROM tools WHERE tenant_id=$1 AND name=$2 ORDER BY version DESC LIMIT 1`,
		tenant, name))
	if err != nil {
		return domain.ToolDefinition{}, false
	}
	return def, true
}

func (p *Postgres) DeleteTool(tenant, name string) error {
	_, err := p.db.Exec(`DELETE FROM tools WHERE tenant_id=$1 AND name=$2`, tenant, name)
	return err
}

func (p *Postgres) ListTools(tenant string, filter domain.ToolFilter) []domain.ToolDefinition {
	query := `SELECT name,version,kind,tenant_id,input_schema,output_schema,transport,endpoint,auth,rbac,tags FROM tools WHERE tenant_id=$1`
	args := []any{tenant}
	argn := 2
	if filter.Kind != "" {
		query += fmt.Sprintf(" AND kind=$%d", argn)
		args = append(args, filter.Kind)
		argn++
	}
	if len(filter.Tags) > 0 {
		for _, tag := range filter.Tags {
			query += fmt.Sprintf(" AND $%d = ANY(tags)", argn)
			args = append(args, tag)
			argn++
		}
	}
	query += " ORDER BY name"
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.ToolDefinition
	for rows.Next() {
		def, err := rowToTool(rows)
		if err != nil {
			continue
		}
		out = append(out, def)
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out
}

func (p *Postgres) SaveSkill(s domain.Skill) error {
	mJSON, _ := json.Marshal(s.Manifest)
	_, err := p.db.Exec(`INSERT INTO skills (id,tenant_id,name,version,manifest) VALUES ($1,$2,$3,$4,$5::jsonb) ON CONFLICT (id) DO UPDATE SET manifest=EXCLUDED.manifest`,
		s.ID, s.TenantID, s.Name, s.Version, string(mJSON))
	return err
}

func (p *Postgres) GetSkill(tenant, name, version string) (domain.Skill, bool) {
	var s domain.Skill
	var mJSON string
	err := p.db.QueryRow(`SELECT id,tenant_id,name,version,manifest FROM skills WHERE tenant_id=$1 AND name=$2 AND version=$3`, tenant, name, version).Scan(&s.ID, &s.TenantID, &s.Name, &s.Version, &mJSON)
	if err != nil {
		return domain.Skill{}, false
	}
	json.Unmarshal([]byte(mJSON), &s.Manifest)
	return s, true
}

func (p *Postgres) ListSkills(tenant, version string) []domain.Skill {
	query := `SELECT id,tenant_id,name,version,manifest FROM skills WHERE tenant_id=$1`
	args := []any{tenant}
	if version != "" {
		query += " AND version=$2"
		args = append(args, version)
	}
	query += " ORDER BY name"
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Skill
	for rows.Next() {
		var s domain.Skill
		var mJSON string
		rows.Scan(&s.ID, &s.TenantID, &s.Name, &s.Version, &mJSON)
		json.Unmarshal([]byte(mJSON), &s.Manifest)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (p *Postgres) SaveRule(r domain.Rule) error {
	_, err := p.db.Exec(`INSERT INTO rules (id,tenant_id,name,policy_rego,severity) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO UPDATE SET policy_rego=EXCLUDED.policy_rego, severity=EXCLUDED.severity`,
		r.ID, r.TenantID, r.Name, r.PolicyRego, r.Severity)
	return err
}

func (p *Postgres) GetRule(tenant, name string) (domain.Rule, bool) {
	var r domain.Rule
	err := p.db.QueryRow(`SELECT id,tenant_id,name,policy_rego,severity FROM rules WHERE tenant_id=$1 AND name=$2`, tenant, name).Scan(&r.ID, &r.TenantID, &r.Name, &r.PolicyRego, &r.Severity)
	if err != nil {
		return domain.Rule{}, false
	}
	return r, true
}

func (p *Postgres) ListRules(tenant string) []domain.Rule {
	rows, err := p.db.Query(`SELECT id,tenant_id,name,policy_rego,severity FROM rules WHERE tenant_id=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Rule
	for rows.Next() {
		var r domain.Rule
		rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.PolicyRego, &r.Severity)
		out = append(out, r)
	}
	return out
}

func (p *Postgres) SaveSpec(s domain.Spec) error {
	_, err := p.db.Exec(`INSERT INTO specs (id,tenant_id,name,agent_spec_ref) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO UPDATE SET agent_spec_ref=EXCLUDED.agent_spec_ref`,
		s.ID, s.TenantID, s.Name, s.AgentSpecRef)
	return err
}

func (p *Postgres) GetSpec(tenant, name string) (domain.Spec, bool) {
	var s domain.Spec
	err := p.db.QueryRow(`SELECT id,tenant_id,name,agent_spec_ref FROM specs WHERE tenant_id=$1 AND name=$2`, tenant, name).Scan(&s.ID, &s.TenantID, &s.Name, &s.AgentSpecRef)
	if err != nil {
		return domain.Spec{}, false
	}
	return s, true
}

func (p *Postgres) ListSpecs(tenant string) []domain.Spec {
	rows, err := p.db.Query(`SELECT id,tenant_id,name,agent_spec_ref FROM specs WHERE tenant_id=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []domain.Spec
	for rows.Next() {
		var s domain.Spec
		rows.Scan(&s.ID, &s.TenantID, &s.Name, &s.AgentSpecRef)
		out = append(out, s)
	}
	return out
}

var _ domain.Store = (*Postgres)(nil)
