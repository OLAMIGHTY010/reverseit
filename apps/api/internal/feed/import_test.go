package feed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

type irig struct {
	im     *Importer
	tenant uuid.UUID
	t      *testing.T
}

func newIRig(t *testing.T) *irig {
	pool := dbtest.Pool(t)
	r := &irig{im: &Importer{Pool: pool, Txn: &txn.Service{Now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }}},
		tenant: dbtest.Tenant(t, pool), t: t}
	r.activate(switchA())
	return r
}

func (r *irig) activate(m *Mapping) {
	r.t.Helper()
	err := r.im.Pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		if err := Save(context.Background(), tx, r.tenant, m, "test"); err != nil {
			return err
		}
		return Activate(context.Background(), tx, r.tenant, m.Name, m.Version)
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *irig) count(q string, args ...any) (n int) {
	r.t.Helper()
	err := r.im.Pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), q, args...).Scan(&n)
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return
}

func TestImportIngestsGoodRowsAndQuarantinesTheRest(t *testing.T) {
	r := newIRig(t)
	res, err := r.im.Import(context.Background(), r.tenant, "switch-a-csv", fixture(t, "switch_a.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 || res.Accepted != 2 || res.Rejected != 4 || res.Status != "imported_with_errors" || res.AlreadySeen {
		t.Fatalf("%+v", res)
	}
	if n := r.count(`SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant); n != 2 {
		t.Fatalf("%d events stored", n)
	}
	if n := r.count(`SELECT count(*) FROM quarantined_events WHERE tenant_id = $1 AND reason LIKE 'feed switch-a-csv v1 line %'`, r.tenant); n != 4 {
		t.Fatalf("%d quarantined", n)
	}
	// The two good rows are one transaction that timed out. A timeout is not a failure.
	var state string
	if err := r.im.Pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT lifecycle_state FROM transactions WHERE tenant_id = $1 AND instruction_ref = 'INS-1001'`, r.tenant).Scan(&state)
	}); err != nil || state != "UNKNOWN" {
		t.Fatalf("%q %v", state, err)
	}
}

func TestTheSameFileIsImportedOnceAndAReplayChangesNothing(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	first, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv"))
	if err != nil || !again.AlreadySeen || again.FileID != first.FileID || again.Accepted != first.Accepted {
		t.Fatalf("%+v %v", again, err)
	}
	if r.count(`SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant) != 2 ||
		r.count(`SELECT count(*) FROM quarantined_events WHERE tenant_id = $1`, r.tenant) != 4 ||
		r.count(`SELECT count(*) FROM feed_files WHERE tenant_id = $1`, r.tenant) != 1 {
		t.Fatal("a repeated upload changed stored data")
	}
}

func TestDifferentBytesWithTheSameEventsAreDuplicatesNotNewEvidence(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	if _, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv")); err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(fixture(t, "switch_a.csv")), "\n", 4)
	resent := lines[0] + "\n" + lines[1] + "\n" + lines[2] + "\n" // the two good rows again, as a new file
	res, err := r.im.Import(ctx, r.tenant, "switch-a-csv", []byte(resent+"\n"))
	if err != nil || res.AlreadySeen || res.Duplicate != 2 || res.Accepted != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if r.count(`SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant) != 2 {
		t.Fatal("a resent event was stored twice")
	}
}

func TestAReusedEventReferenceWithDifferentContentIsRefusedAndKept(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	if _, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv")); err != nil {
		t.Fatal(err)
	}
	altered := strings.Replace(string(fixture(t, "switch_a.csv")), "18500.00,NGN,acct_0123456789abcdef0123456789abcdef,TEST BANK,acct_fedcba9876543210fedcba9876543210,SESS-1001,RRN000000001,000101\nEV-1002", "99999.00,NGN,acct_0123456789abcdef0123456789abcdef,TEST BANK,acct_fedcba9876543210fedcba9876543210,SESS-1001,RRN000000001,000101\nEV-1002", 1)
	res, err := r.im.Import(ctx, r.tenant, "switch-a-csv", []byte(altered))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, re := range res.RowErrors {
		if strings.Contains(re.Reason, "different content") || strings.Contains(re.Reason, "conflicts") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a changed event was not refused: %+v", res)
	}
	var amount int64
	_ = r.im.Pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT amount_minor FROM transactions WHERE tenant_id = $1 AND instruction_ref = 'INS-1001'`, r.tenant).Scan(&amount)
	})
	if amount != 1850000 {
		t.Fatalf("the stored amount changed to %d", amount)
	}
}

func TestMappingsAreVersionedImmutableAndOneIsActive(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	v1 := switchA()
	err := r.im.Pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Save(ctx, tx, r.tenant, v1, "x") })
	if !errors.Is(err, ErrMappingExists) {
		t.Fatalf("a stored version was overwritten: %v", err)
	}
	v2 := switchA()
	v2.Version = 2
	v2.Fields[FRail] = Locator{Column: "rail"}
	r.activate(v2)
	if n := r.count(`SELECT count(*) FROM feed_mappings WHERE tenant_id = $1 AND status = 'active'`, r.tenant); n != 1 {
		t.Fatalf("%d active", n)
	}
	err = r.im.Pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Activate(ctx, tx, r.tenant, "switch-a-csv", 1) })
	if err == nil {
		t.Fatal("a retired version was reactivated")
	}
	err = r.im.Pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Activate(ctx, tx, r.tenant, "nope", 1) })
	if !errors.Is(err, ErrNoMapping) {
		t.Fatalf("%v", err)
	}
	if _, err := r.im.Import(ctx, r.tenant, "unknown", []byte("x")); !errors.Is(err, ErrNoMapping) {
		t.Fatalf("%v", err)
	}
	res, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv"))
	if err != nil || res.Version != 2 {
		t.Fatalf("the import did not use the active version: %+v %v", res, err)
	}
}

func TestOneInstitutionsMappingsAndFilesAreInvisibleToAnother(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	if _, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv")); err != nil {
		t.Fatal(err)
	}
	other := dbtest.Tenant(t, r.im.Pool)
	if _, err := r.im.Import(ctx, other, "switch-a-csv", fixture(t, "switch_a.csv")); !errors.Is(err, ErrNoMapping) {
		t.Fatalf("another institution used this one's mapping: %v", err)
	}
	var n int
	_ = r.im.Pool.WithTenant(ctx, other, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM feed_files`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("%d files visible", n)
	}
}

func TestOversizeAndUnparseableFilesAreRefusedWholeAndLeaveNoTrace(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	if _, err := r.im.Import(ctx, r.tenant, "switch-a-csv", make([]byte, MaxFileBytes+1)); err == nil {
		t.Fatal("an oversize file was accepted")
	}
	if _, err := r.im.Import(ctx, r.tenant, "switch-a-csv", []byte("a,b\n1,2\n")); err == nil {
		t.Fatal("a file without the mapped columns was accepted")
	}
	if r.count(`SELECT count(*) FROM feed_files WHERE tenant_id = $1`, r.tenant) != 0 || r.count(`SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant) != 0 {
		t.Fatal("a refused file left data behind")
	}
}

func TestConcurrentUploadsOfTheSameFileStoreEachEventOnceAndOneFileRecord(t *testing.T) {
	r := newIRig(t)
	ctx := context.Background()
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := r.im.Import(ctx, r.tenant, "switch-a-csv", fixture(t, "switch_a.csv"))
			done <- err
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	if r.count(`SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant) != 2 || r.count(`SELECT count(*) FROM feed_files WHERE tenant_id = $1`, r.tenant) != 1 {
		t.Fatal("concurrent uploads duplicated data")
	}
}
