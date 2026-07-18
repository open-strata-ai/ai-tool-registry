package store_test

import (
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/domain"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/store"
)

func TestStoreLatestVersion(t *testing.T) {
	m := store.New()
	_ = m.SaveTool(domain.ToolDefinition{Name: "t", Version: "1.0.0", TenantID: "A", Kind: domain.KindAPI, Transport: domain.TransportHTTP})
	_ = m.SaveTool(domain.ToolDefinition{Name: "t", Version: "1.2.0", TenantID: "A", Kind: domain.KindAPI, Transport: domain.TransportHTTP})
	_ = m.SaveTool(domain.ToolDefinition{Name: "t", Version: "0.9.0", TenantID: "A", Kind: domain.KindAPI, Transport: domain.TransportHTTP})

	def, ok := m.GetLatestTool("A", "t")
	if !ok || def.Version != "1.2.0" {
		t.Fatalf("want 1.2.0, got %+v ok=%v", def, ok)
	}
}

func TestStoreDeleteAndList(t *testing.T) {
	m := store.New()
	_ = m.SaveTool(domain.ToolDefinition{Name: "a", Version: "1.0.0", TenantID: "A", Kind: domain.KindAPI, Transport: domain.TransportHTTP, CapabilityTags: []string{"erp"}})
	_ = m.SaveTool(domain.ToolDefinition{Name: "b", Version: "1.0.0", TenantID: "A", Kind: domain.KindDB, Transport: domain.TransportLocal})

	if err := m.DeleteTool("A", "a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := m.GetLatestTool("A", "a"); ok {
		t.Fatalf("expected a deleted")
	}

	apiOnly := m.ListTools("A", domain.ToolFilter{Kind: domain.KindAPI})
	if len(apiOnly) != 0 {
		t.Fatalf("want 0 api tools after delete, got %d", len(apiOnly))
	}
	tagged := m.ListTools("A", domain.ToolFilter{Tags: []string{"erp"}})
	if len(tagged) != 0 {
		t.Fatalf("want 0 (tagged tool deleted), got %d", len(tagged))
	}
}

func TestStoreSkillsRulesSpecs(t *testing.T) {
	m := store.New()
	_ = m.SaveSkill(domain.Skill{Name: "s", Version: "1.0.0", TenantID: "A"})
	_ = m.SaveRule(domain.Rule{Name: "r", TenantID: "A"})
	_ = m.SaveSpec(domain.Spec{Name: "sp", TenantID: "A"})

	if len(m.ListSkills("A", "")) != 1 {
		t.Fatalf("want 1 skill")
	}
	if len(m.ListRules("A")) != 1 {
		t.Fatalf("want 1 rule")
	}
	if len(m.ListSpecs("A")) != 1 {
		t.Fatalf("want 1 spec")
	}
}
