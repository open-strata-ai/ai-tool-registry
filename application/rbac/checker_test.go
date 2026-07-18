package rbac_test

import (
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/application/rbac"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

func TestAllow(t *testing.T) {
	c := rbac.New()
	tool := domain.ToolDefinition{RBAC: []string{"agent"}}

	if !c.Allow(tool, "agent") {
		t.Fatalf("agent should be allowed")
	}
	if c.Allow(tool, "member") {
		t.Fatalf("member should be denied")
	}
	if !c.Allow(tool, "admin") {
		t.Fatalf("admin should bypass RBAC")
	}
	if !c.Allow(tool, "ADMIN") {
		t.Fatalf("role should be case-insensitive")
	}
}

func TestAllowEmptyListPermitsAll(t *testing.T) {
	c := rbac.New()
	if !c.Allow(domain.ToolDefinition{}, "anonymous") {
		t.Fatalf("empty RBAC should permit all roles")
	}
}
