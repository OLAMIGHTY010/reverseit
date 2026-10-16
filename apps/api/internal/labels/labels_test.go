package labels

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const tok = "acct_0123456789abcdef0123456789abcdef"

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	svc    *cases.Service
	ing    *txn.Service
	n      int
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	app := &audit.Appender{}
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), ing: &txn.Service{}, t: t}
	r.svc = &cases.Service{Audit: app, OnClose: OnClose}
	return r
}

func (r *rig) in(fn func(tx pgx.Tx) error) {
	r.t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, fn); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) txn() uuid.UUID {
	r.n++
	var id uuid.UUID
	r.in(func(tx pgx.Tx) error {
		res, err := r.ing.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-LB-%d", r.n), EventType: txn.EvDebitPosted,
			SourceTS: time.Now().Add(-time.Hour), Txn: txn.Core{InstructionRef: fmt.Sprintf("INS-LB%d", r.n), Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tok}})
		id = res.TxnID
		return err
	})
	return id
}

func (r *rig) open(source string, txnID uuid.UUID) cases.Case {
	var c cases.Case
	r.in(func(tx pgx.Tx) error {
		var err error
		code := cases.CodeCustomer
		if source == cases.SourceBot {
			code = cases.CodeNoCredit
		}
		c, _, err = r.svc.Open(context.Background(), tx, r.tenant, cases.OpenInput{Classification: code, Source: source, TxnID: &txnID, Rail: "NIP", RuleID: "T", RuleVersion: "1", Explanation: "test", Actor: cases.System})
		return err
	})
	return c
}

func (r *rig) close(c cases.Case, by uuid.UUID) {
	r.in(func(tx pgx.Tx) error {
		a := cases.Actor{Type: audit.ActorUser, ID: by}
		for _, to := range []string{cases.StUnderInvestigation, cases.StClosedNoException} {
			if _, err := r.svc.Transition(context.Background(), tx, r.tenant, c.ID, a, to, 0, "CHECKED"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *rig) list(status string) []Example {
	var out []Example
	r.in(func(tx pgx.Tx) error {
		var err error
		out, err = List(context.Background(), tx, r.tenant, status)
		return err
	})
	return out
}

func TestAClosedCustomerCaseThatTheDetectorMissedBecomesAPendingExampleAndOthersDoNot(t *testing.T) {
	r := newRig(t)
	closer := uuid.New()
	missed := r.open(cases.SourceCustomer, r.txn())
	r.close(missed, closer)
	if got := r.list("pending"); len(got) != 1 || got[0].CaseID != missed.ID || got[0].Version != 1 {
		t.Fatalf("%+v", got)
	}

	// The detector had raised this one: nothing to learn.
	tid := r.txn()
	r.open(cases.SourceBot, tid)
	caught := r.open(cases.SourceCustomer, tid)
	r.close(caught, closer)
	if got := r.list("pending"); len(got) != 1 {
		t.Fatalf("a case the detector already had became an example: %d", len(got))
	}
	// A case with no transaction has nothing to label.
	var unmatched cases.Case
	r.in(func(tx pgx.Tx) error {
		var err error
		unmatched, _, err = r.svc.Open(context.Background(), tx, r.tenant, cases.OpenInput{Classification: cases.CodeCustomer, Source: cases.SourceCustomer, RuleID: "T", RuleVersion: "1", Explanation: "t", Actor: cases.System})
		return err
	})
	r.close(unmatched, closer)
	if got := r.list("pending"); len(got) != 1 {
		t.Fatalf("%d", len(got))
	}
	// A bot case closing makes no example either.
	b := r.open(cases.SourceBot, r.txn())
	r.close(b, closer)
	if got := r.list("pending"); len(got) != 1 {
		t.Fatalf("%d", len(got))
	}
}

func TestOnlyAConfirmedExampleMayBeUsedAndTheCloserMayNotConfirmIt(t *testing.T) {
	r := newRig(t)
	closer, reviewer := uuid.New(), uuid.New()
	c := r.open(cases.SourceCustomer, r.txn())
	r.close(c, closer)
	ex := r.list("pending")[0]
	decide := func(by uuid.UUID, confirm bool, note string) (Example, error) {
		var e Example
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			var err error
			e, err = Decide(context.Background(), tx, r.tenant, by, ex.ID, confirm, note, time.Now())
			return err
		})
		return e, err
	}
	confirmed := func() int {
		var n int
		r.in(func(tx pgx.Tx) error { l, err := Confirmed(context.Background(), tx, r.tenant); n = len(l); return err })
		return n
	}
	if confirmed() != 0 {
		t.Fatal("a pending example is usable")
	}
	if _, err := decide(closer, true, ""); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("the closer confirmed their own label: %v", err)
	}
	if _, err := decide(reviewer, false, ""); err == nil {
		t.Fatal("a rejection with no note")
	}
	next, err := decide(reviewer, true, "root cause is clear")
	if err != nil || next.Status != "confirmed" || next.Version != 2 || next.ID == ex.ID {
		t.Fatalf("%+v %v", next, err)
	}
	if confirmed() != 1 || len(r.list("pending")) != 0 || len(r.list("superseded")) != 1 {
		t.Fatalf("confirmed %d pending %d superseded %d", confirmed(), len(r.list("pending")), len(r.list("superseded")))
	}
	if _, err := decide(reviewer, true, "again"); !errors.Is(err, ErrState) {
		t.Fatalf("a decided example was decided again: %v", err)
	}
	if _, err := decide(reviewer, true, "x"); err == nil {
		t.Fatal("expected a state error for the superseded version")
	}
	other := dbtest.Tenant(t, r.pool)
	_ = r.pool.WithTenant(context.Background(), other, func(tx pgx.Tx) error {
		l, _ := Confirmed(context.Background(), tx, other)
		if len(l) != 0 {
			t.Errorf("%d examples visible to another institution", len(l))
		}
		return nil
	})
}

func TestARejectedExampleIsKeptAndNeverUsed(t *testing.T) {
	r := newRig(t)
	c := r.open(cases.SourceCustomer, r.txn())
	r.close(c, uuid.New())
	ex := r.list("pending")[0]
	r.in(func(tx pgx.Tx) error {
		_, err := Decide(context.Background(), tx, r.tenant, uuid.New(), ex.ID, false, "the customer was mistaken", time.Now())
		return err
	})
	if len(r.list("rejected")) != 1 || len(r.list("confirmed")) != 0 {
		t.Fatal("a rejected example should be kept and unusable")
	}
}
