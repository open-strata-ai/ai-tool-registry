// Package store is an in-memory domain.Store (production: PostgreSQL +
// Redis L2 hot copy, DESIGN §8 / §6.5). It is thread-safe and used by tests and
// the offline bootstrap in cmd/main.go.
package store

import (
	"sort"
	"strings"
	"sync"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// InMemory is a thread-safe Store.
type InMemory struct {
	mu     sync.RWMutex
	tools  map[string]domain.ToolDefinition
	skills map[string]domain.Skill
	rules  map[string]domain.Rule
	specs  map[string]domain.Spec
}

// New builds an empty store.
func New() *InMemory {
	return &InMemory{
		tools:  map[string]domain.ToolDefinition{},
		skills: map[string]domain.Skill{},
		rules:  map[string]domain.Rule{},
		specs:  map[string]domain.Spec{},
	}
}

func key(tenant, name, version string) string { return tenant + "/" + name + "/" + version }

// --- tools ---

func (m *InMemory) SaveTool(def domain.ToolDefinition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[key(def.TenantID, def.Name, def.Version)] = def
	return nil
}

func (m *InMemory) GetTool(tenant, name, version string) (domain.ToolDefinition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.tools[key(tenant, name, version)]
	return d, ok
}

func (m *InMemory) GetLatestTool(tenant, name string) (domain.ToolDefinition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var best domain.ToolDefinition
	bestV := ""
	found := false
	for _, d := range m.tools {
		if d.TenantID != tenant || d.Name != name {
			continue
		}
		if bestV == "" || compareVersion(d.Version, bestV) > 0 {
			best = d
			bestV = d.Version
			found = true
		}
	}
	return best, found
}

func (m *InMemory) DeleteTool(tenant, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, d := range m.tools {
		if d.TenantID == tenant && d.Name == name {
			delete(m.tools, k)
		}
	}
	return nil
}

func (m *InMemory) ListTools(tenant string, filter domain.ToolFilter) []domain.ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.ToolDefinition
	for _, d := range m.tools {
		if d.TenantID != tenant {
			continue
		}
		if filter.Kind != "" && d.Kind != filter.Kind {
			continue
		}
		if len(filter.Tags) > 0 && !hasAnyTag(d.CapabilityTags, filter.Tags) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out
}

// --- skills ---

func (m *InMemory) SaveSkill(s domain.Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.skills[key(s.TenantID, s.Name, s.Version)] = s
	return nil
}

func (m *InMemory) GetSkill(tenant, name, version string) (domain.Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.skills[key(tenant, name, version)]
	return s, ok
}

func (m *InMemory) ListSkills(tenant, version string) []domain.Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Skill
	for _, s := range m.skills {
		if s.TenantID != tenant {
			continue
		}
		if version != "" && s.Version != version {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- rules ---

func (m *InMemory) SaveRule(r domain.Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[key(r.TenantID, r.Name, "")] = r
	return nil
}

func (m *InMemory) GetRule(tenant, name string) (domain.Rule, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rules[key(tenant, name, "")]
	return r, ok
}

func (m *InMemory) ListRules(tenant string) []domain.Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Rule
	for _, r := range m.rules {
		if r.TenantID == tenant {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- specs ---

func (m *InMemory) SaveSpec(s domain.Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.specs[key(s.TenantID, s.Name, "")] = s
	return nil
}

func (m *InMemory) GetSpec(tenant, name string) (domain.Spec, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.specs[key(tenant, name, "")]
	return s, ok
}

func (m *InMemory) ListSpecs(tenant string) []domain.Spec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Spec
	for _, s := range m.specs {
		if s.TenantID == tenant {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func hasAnyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(h, w) {
				return true
			}
		}
	}
	return false
}

// compareVersion compares two semantic-ish versions; returns -1/0/1.
func compareVersion(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai = atoiSafe(as[i])
		}
		if i < len(bs) {
			bi = atoiSafe(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

func atoiSafe(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int(c-'0')
	}
	return v
}

var _ domain.Store = (*InMemory)(nil)
