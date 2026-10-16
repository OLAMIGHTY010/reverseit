// Package contract holds tests that tie the code to packages/api-contracts.
package contract

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// specPath locates openapi.yaml relative to this file.
func specPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "packages", "api-contracts", "openapi.yaml")
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(specPath(t))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	return doc
}

func TestSpecIsValidOpenAPI(t *testing.T) {
	if err := loadSpec(t).Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml is not valid: %v", err)
	}
}

func TestEveryOperationDeclaresStatusAndPhase(t *testing.T) {
	doc := loadSpec(t)
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			status, _ := op.Extensions["x-status"].(string)
			switch status {
			case "implemented":
			case "planned":
				if phase, _ := op.Extensions["x-phase"].(string); phase == "" {
					t.Errorf("%s %s is planned but has no x-phase", method, path)
				}
			default:
				t.Errorf("%s %s has x-status %q; want implemented or planned", method, path, status)
			}
			if op.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
			}
		}
	}
}

func TestEveryPathIsVersioned(t *testing.T) {
	for path := range loadSpec(t).Paths.Map() {
		if len(path) < 4 || path[:4] != "/v1/" {
			t.Errorf("path %q is not under /v1", path)
		}
	}
}
