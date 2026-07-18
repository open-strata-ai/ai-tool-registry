// Package rbac implements tool-level authorization (DESIGN §5.1 / R4). It wraps
// domain.RoleAllowed and adds the platform "admin" bypass and role normalization.
package rbac

import (
	"strings"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Checker resolves whether a caller role may use a tool.
type Checker struct{}

// New builds a Checker.
func New() *Checker { return &Checker{} }

// Allow reports whether callerRole satisfies tool.RBAC. The "admin" role always
// passes (control-plane superuser); an empty RBAC allow list permits all roles.
func (c *Checker) Allow(tool domain.ToolDefinition, callerRole string) bool {
	role := normalize(callerRole)
	if role == "admin" {
		return true
	}
	return domain.RoleAllowed(tool.RBAC, role)
}

func normalize(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

var _ = domain.RoleAllowed
