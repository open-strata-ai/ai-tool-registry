package schema_test

import (
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/application/schema"
)

func TestValidateRequired(t *testing.T) {
	v := schema.New()
	sch := map[string]any{
		"type":       "object",
		"required":   []any{"order_id"},
		"properties": map[string]any{"order_id": map[string]any{"type": "string"}},
	}
	if err := v.Validate(sch, map[string]any{}); err == nil {
		t.Fatalf("expected missing required error")
	}
	if err := v.Validate(sch, map[string]any{"order_id": "O1"}); err != nil {
		t.Fatalf("valid input should pass: %v", err)
	}
}

func TestValidateType(t *testing.T) {
	v := schema.New()
	sch := map[string]any{
		"type":       "object",
		"properties": map[string]any{"amount": map[string]any{"type": "number"}},
	}
	if err := v.Validate(sch, map[string]any{"amount": "notnum"}); err == nil {
		t.Fatalf("expected type error")
	}
	if err := v.Validate(sch, map[string]any{"amount": 12.5}); err != nil {
		t.Fatalf("number should pass: %v", err)
	}
}

func TestValidateEnum(t *testing.T) {
	v := schema.New()
	sch := map[string]any{
		"type":       "object",
		"properties": map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}},
	}
	if err := v.Validate(sch, map[string]any{"mode": "c"}); err == nil {
		t.Fatalf("expected enum error")
	}
	if err := v.Validate(sch, map[string]any{"mode": "a"}); err != nil {
		t.Fatalf("enum value should pass: %v", err)
	}
}

func TestValidateEmptySchemaPasses(t *testing.T) {
	v := schema.New()
	if err := v.Validate(nil, map[string]any{"anything": 1}); err != nil {
		t.Fatalf("empty schema should pass: %v", err)
	}
}
