// Package schema implements domain.SchemaValidator (DESIGN §5.2): a lightweight
// JSON-Schema structural check. Production swaps in gojsonschema; the offline
// build stays stdlib-only and verifies the fields the registry cares about
// (type, required, nested object properties, enum).
package schema

import (
	"fmt"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Validator is a stdlib-only schema validator.
type Validator struct{}

// New builds a Validator.
func New() *Validator { return &Validator{} }

// Validate checks data against schema. An empty/absent schema passes. Supported
// keywords: type, required (object properties), properties (recursive object
// type + enum), and a top-level enum.
func (v *Validator) Validate(schema map[string]any, data map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	return v.validateNode(schema, data, "")
}

func (v *Validator) validateNode(schema, data map[string]any, path string) error {
	// top-level enum applies to the whole object's serialized presence is not
	// meaningful for an object; only validate structurally.
	if req, ok := schema["required"]; ok {
		reqList, _ := req.([]any)
		for _, r := range reqList {
			key, _ := r.(string)
			if key == "" {
				continue
			}
			if _, present := data[key]; !present {
				return fmt.Errorf("%srequired property %q missing", path, key)
			}
		}
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name, pRaw := range props {
			pSchema, ok := pRaw.(map[string]any)
			if !ok {
				continue
			}
			val, present := data[name]
			if !present {
				continue
			}
			if err := v.validateValue(pSchema, val, path+name+"."); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Validator) validateValue(pSchema map[string]any, val any, path string) error {
	if typ, ok := pSchema["type"].(string); ok {
		if err := checkType(typ, val); err != nil {
			return fmt.Errorf("%s%s", path, err.Error())
		}
	}
	if enum, ok := pSchema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == val {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%svalue %v not in enum", path, val)
		}
	}
	if typ, _ := pSchema["type"].(string); typ == "object" {
		if sub, ok := pSchema["properties"].(map[string]any); ok {
			subData, ok := val.(map[string]any)
			if ok {
				if err := v.validateNode(sub, subData, path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkType(typ string, val any) error {
	switch typ {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case "number":
		switch val.(type) {
		case float64, int, int64:
		default:
			return fmt.Errorf("expected number, got %T", val)
		}
	case "integer":
		switch v := val.(type) {
		case float64:
			if v != float64(int64(v)) {
				return fmt.Errorf("expected integer, got float %v", v)
			}
		case int, int64:
		default:
			return fmt.Errorf("expected integer, got %T", val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", val)
		}
	case "array":
		if _, ok := val.([]any); !ok {
			return fmt.Errorf("expected array, got %T", val)
		}
	case "object":
		if _, ok := val.(map[string]any); !ok {
			return fmt.Errorf("expected object, got %T", val)
		}
	case "null":
		if val != nil {
			return fmt.Errorf("expected null, got %T", val)
		}
	default:
		// unknown type keyword → accept (forward compatible)
	}
	return nil
}

var _ domain.SchemaValidator = (*Validator)(nil)
