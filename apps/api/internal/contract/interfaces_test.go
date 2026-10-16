package contract

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/smartreversal/smartreversal/apps/api/internal/mocks"
)

// interfaceFiles are the six external interfaces in scope section 13.3.
var interfaceFiles = []string{
	"transaction-feed.yaml",
	"status-query.yaml",
	"reversal-api.yaml",
	"settlement-files.yaml",
	"notification-gateway.yaml",
	"customer-app.yaml",
}

func interfacesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(filepath.Dir(specPath(t)), "interfaces")
}

func TestAllSixInterfaceContractsExistAndAreValid(t *testing.T) {
	dir := interfacesDir(t)
	for _, name := range interfaceFiles {
		t.Run(name, func(t *testing.T) {
			doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := doc.Validate(context.Background()); err != nil {
				t.Fatalf("invalid: %v", err)
			}
			if doc.Paths.Len() == 0 {
				t.Fatal("no paths")
			}
			for path, item := range doc.Paths.Map() {
				for method, op := range item.Operations() {
					if op.OperationID == "" {
						t.Errorf("%s %s has no operationId", method, path)
					}
					if s, _ := op.Extensions["x-status"].(string); s != "planned" && s != "implemented" {
						t.Errorf("%s %s has x-status %q", method, path, s)
					}
				}
			}
		})
	}
}

// A seventh file would be an interface nobody agreed to review.
func TestNoUnlistedInterfaceFiles(t *testing.T) {
	entries, err := os.ReadDir(interfacesDir(t))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"README.md": true}
	for _, n := range interfaceFiles {
		known[n] = true
	}
	for _, e := range entries {
		if !known[e.Name()] {
			t.Errorf("interfaces/%s is not in the list of agreed interfaces", e.Name())
		}
	}
}

// The mock stubs must expose exactly the operations the contracts define, so a
// mock cannot drift from the interface it stands in for.
func TestMockRoutesMatchInterfaceContracts(t *testing.T) {
	byKind := map[mocks.Kind]string{
		mocks.KindStatus:   "status-query.yaml",
		mocks.KindReversal: "reversal-api.yaml",
		mocks.KindNotify:   "notification-gateway.yaml",
	}
	for kind, file := range byKind {
		doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(interfacesDir(t), file))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]bool{}
		for path, item := range doc.Paths.Map() {
			for method := range item.Operations() {
				want[method+" "+path] = true
			}
		}
		got := map[string]bool{}
		for _, op := range mocks.Routes(kind) {
			got[op] = true
		}
		if missing := diff(want, got); len(missing) > 0 {
			t.Errorf("%s mock lacks operations from %s: %v", kind, file, missing)
		}
		if extra := diff(got, want); len(extra) > 0 {
			t.Errorf("%s mock has operations not in %s: %v", kind, file, extra)
		}
	}
}
