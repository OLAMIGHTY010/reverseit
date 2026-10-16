package reversal

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
)

func TestGateOverrideValidation(t *testing.T) {
	ctx := context.Background()
	svc := &Service{}

	// Propose without operator
	_, err := svc.ProposeGateOverride(ctx, uuid.New(), uuid.New(), nil, "Short")
	if !errors.Is(err, cases.ErrInvalid) {
		t.Fatalf("expected ErrInvalid for short justification and nil principal, got %v", err)
	}

	// Justification too short
	p := &identity.Principal{UserID: uuid.New(), Roles: []string{"SUPERVISOR"}}
	_, err = svc.ProposeGateOverride(ctx, uuid.New(), uuid.New(), p, "short")
	if !errors.Is(err, cases.ErrInvalid) {
		t.Fatalf("expected ErrInvalid for short justification, got %v", err)
	}

	// Decide with same actor
	decider := &identity.Principal{UserID: uuid.New(), Roles: []string{"SUPERVISOR"}, MFA: true}
	// Missing MFA
	deciderNoMFA := &identity.Principal{UserID: uuid.New(), Roles: []string{"SUPERVISOR"}, MFA: false}
	pol := &authz.Policy{}
	svc.Policy = pol

	_, err = svc.DecideGateOverride(ctx, uuid.New(), uuid.New(), deciderNoMFA, true, "Valid reason here")
	if err == nil {
		t.Fatal("expected error for missing MFA or policy check")
	}

	_ = decider
}
