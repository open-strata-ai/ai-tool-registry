// Package config loads tool-registry configuration (DESIGN §11.5). To keep the
// build stdlib-only it ships typed defaults and an optional JSON overlay;
// production renders infrastructure/config/registry.yaml via the meta repository.
package config

import (
	"encoding/json"
	"os"
)

// Config mirrors the DESIGN §11.5 config keys.
type Config struct {
	ToolRegistry struct {
		SchemaValidation bool
		Discovery        struct {
			Enabled      bool   `json:"enabled"`
			HeartbeatTTL string `json:"heartbeatTTL"`
		} `json:"discovery"`
		MCP struct {
			Transports []string `json:"transports"`
		} `json:"mcp"`
		Metering struct {
			Enabled bool `json:"enabled"`
		} `json:"metering"`
	} `json:"toolRegistry"`
	Auth struct {
		Provider string `json:"provider"`
	} `json:"auth"`
	Skills struct {
		Enabled bool `json:"enabled"`
	} `json:"skills"`
	Rules struct {
		Enabled bool `json:"enabled"`
	} `json:"rules"`
	Specs struct {
		Enabled bool `json:"enabled"`
	} `json:"specs"`
}

// Default returns the DESIGN §11.5 defaults.
func Default() Config {
	var c Config
	c.ToolRegistry.SchemaValidation = true
	c.ToolRegistry.Discovery.Enabled = true
	c.ToolRegistry.Discovery.HeartbeatTTL = "30s"
	c.ToolRegistry.MCP.Transports = []string{"stdio", "sse", "http"}
	c.ToolRegistry.Metering.Enabled = true
	c.Auth.Provider = "keycloak"
	c.Skills.Enabled = false
	c.Rules.Enabled = false
	c.Specs.Enabled = false
	return c
}

// Load reads a JSON overlay from path and merges it onto the defaults. A missing
// path returns the defaults unchanged.
func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}
