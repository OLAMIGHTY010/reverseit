package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type fakeChecker struct {
	conflict bool
	err      error
}

func (f fakeChecker) Conflicts(context.Context, uuid.UUID, uuid.UUID, Subject) (bool, error) {
	return f.conflict, f.err
}

func TestMakerCannotBeTheChecker(t *testing.T) {
	maker := uuid.New()
	for _, role := range []string{"SUPERVISOR", "HEAD_OPS", "ADMIN_INTEGRATION"} {
		p := &identity.Principal{UserID: maker, TenantID: uuid.New(), Roles: []string{role}, MFA: true}
		err := SoD{Conflicts: fakeChecker{}}.CanApprove(context.Background(), p, maker, Subject{})
		if !errors.Is(err, ErrSameActor) {
			t.Errorf("%s approving their own request: %v", role, err)
		}
	}
}

func TestADifferentCheckerWithNoConflictMayApprove(t *testing.T) {
	p := &identity.Principal{UserID: uuid.New(), TenantID: uuid.New(), Roles: []string{"SUPERVISOR"}}
	if err := (SoD{Conflicts: fakeChecker{}}).CanApprove(context.Background(), p, uuid.New(), Subject{}); err != nil {
		t.Fatal(err)
	}
}

func TestConflictOfInterestBlocksApproval(t *testing.T) {
	p := &identity.Principal{UserID: uuid.New(), TenantID: uuid.New(), Roles: []string{"SUPERVISOR"}}
	err := SoD{Conflicts: fakeChecker{conflict: true}}.CanApprove(context.Background(), p, uuid.New(), Subject{AccountTokens: []string{"t1"}})
	if !errors.Is(err, ErrConflictOfInterest) {
		t.Fatalf("got %v", err)
	}
}

func TestSoDFailsClosed(t *testing.T) {
	p := &identity.Principal{UserID: uuid.New(), TenantID: uuid.New(), Roles: []string{"SUPERVISOR"}}
	maker := uuid.New()
	ctx := context.Background()

	if err := (SoD{Conflicts: fakeChecker{err: errors.New("db down")}}).CanApprove(ctx, p, maker, Subject{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("checker error: %v", err)
	}
	if err := (SoD{}).CanApprove(ctx, p, maker, Subject{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("no checker configured: %v", err)
	}
	if err := (SoD{Conflicts: fakeChecker{}}).CanApprove(ctx, nil, maker, Subject{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("nil principal: %v", err)
	}
	if err := (SoD{Conflicts: fakeChecker{}}).CanApprove(ctx, p, uuid.Nil, Subject{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown maker: %v", err)
	}
	if err := (SoD{Conflicts: fakeChecker{}}).CanApprove(ctx, &identity.Principal{}, maker, Subject{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("empty principal: %v", err)
	}
}

func TestPGConflictCheckerFindsStaffLinksWithinTheTenantOnly(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	a, b := dbtest.Tenant(t, pool), dbtest.Tenant(t, pool)
	store := &identity.PGUserStore{Pool: pool}
	staffA, err := store.Create(ctx, a, "staff@example.test", "Staff", "a long enough passphrase", []string{"SUPERVISOR"})
	if err != nil {
		t.Fatal(err)
	}
	staffB, err := store.Create(ctx, b, "staff@example.test", "Staff", "a long enough passphrase", []string{"SUPERVISOR"})
	if err != nil {
		t.Fatal(err)
	}
	err = pool.WithTenant(ctx, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO staff_account_links (tenant_id, user_id, account_token) VALUES ($1, $2, 'tok_mine')`, a, staffA)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	c := &PGConflictChecker{Pool: pool}
	check := func(tenant, user uuid.UUID, tokens ...string) bool {
		got, err := c.Conflicts(ctx, tenant, user, Subject{AccountTokens: tokens})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !check(a, staffA, "tok_other", "tok_mine") {
		t.Error("linked account not detected")
	}
	if check(a, staffA, "tok_other") {
		t.Error("conflict reported for an unrelated account")
	}
	if check(a, staffA) {
		t.Error("conflict reported with no accounts")
	}
	if check(b, staffB, "tok_mine") {
		t.Error("tenant B staff matched a link that belongs to tenant A")
	}

	// Tenant B cannot create a link for tenant A's staff.
	err = pool.WithTenant(ctx, b, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO staff_account_links (tenant_id, user_id, account_token) VALUES ($1, $2, 'tok_x')`, a, staffA)
		return err
	})
	if err == nil {
		t.Error("tenant B wrote a link into tenant A")
	}

	// The full path through SoD.
	sod := SoD{Conflicts: c}
	p := &identity.Principal{UserID: staffA, TenantID: a, Roles: []string{"SUPERVISOR"}}
	if err := sod.CanApprove(ctx, p, uuid.New(), Subject{AccountTokens: []string{"tok_mine"}}); !errors.Is(err, ErrConflictOfInterest) {
		t.Errorf("end to end: %v", err)
	}
}
