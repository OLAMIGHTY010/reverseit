package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Anchor is the end of a tenant's chain. Copy it somewhere the database cannot
// reach (a write-once store, a signed daily report) and compare later. Without
// an outside copy, someone who can rewrite the whole table and the head can
// produce a consistent forgery.
type Anchor struct {
	TenantID uuid.UUID
	Seq      int64
	Hash     string
}

// VerifyResult is the outcome of checking a chain.
type VerifyResult struct {
	OK      bool
	Checked int64
	// FirstBadSeq and Reason are set when OK is false.
	FirstBadSeq int64
	Reason      string
	Head        Anchor
}

const columns = `chain_seq, id, occurred_at, actor_type, actor_id, action, subject_type, subject_id,
	reason_code, evidence_refs, before_hash, after_hash, policy_version, ip, device,
	sensitive_access, details, prev_hash, record_hash`

func scan(row pgx.Row, tenant uuid.UUID) (Record, error) {
	var (
		r        Record
		actorTyp string
		actorID  *uuid.UUID
		evidence []byte
		details  []byte
	)
	if err := row.Scan(&r.ChainSeq, &r.ID, &r.OccurredAt, &actorTyp, &actorID, &r.Action, &r.SubjectType,
		&r.SubjectID, &r.ReasonCode, &evidence, &r.BeforeHash, &r.AfterHash, &r.PolicyVersion, &r.IP,
		&r.Device, &r.Sensitive, &details, &r.PrevHash, &r.RecordHash); err != nil {
		return Record{}, err
	}
	r.TenantID = tenant
	r.Actor = ActorType(actorTyp)
	if actorID != nil {
		r.ActorID = *actorID
	}
	if err := json.Unmarshal(evidence, &r.EvidenceRefs); err != nil {
		return Record{}, fmt.Errorf("audit: evidence refs: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(details))
	dec.UseNumber()
	if err := dec.Decode(&r.Details); err != nil {
		return Record{}, fmt.Errorf("audit: details: %w", err)
	}
	r.OccurredAt = r.OccurredAt.UTC()
	return r, nil
}

// requireTenantTx makes sure tx is a transaction for tenant. Without this,
// reading another tenant's chain would show zero rows and look like a clean,
// empty chain.
func requireTenantTx(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	var got string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant_id', true), '')`).Scan(&got); err != nil {
		return fmt.Errorf("audit: read tenant context: %w", err)
	}
	if got != tenant.String() {
		return errors.New("audit: the transaction is not scoped to this tenant")
	}
	return nil
}

// Verify walks the tenant's whole chain using tx, a tenant transaction, and
// recomputes every hash. It reports the first record that does not check out:
// a changed field, a removed or inserted row, a reordering, or a chain that ends
// before the stored head.
func Verify(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (VerifyResult, error) {
	if tenant == uuid.Nil {
		return VerifyResult{}, errors.New("audit: tenant id is required")
	}
	if err := requireTenantTx(ctx, tx, tenant); err != nil {
		return VerifyResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM audit_events WHERE tenant_id = $1 ORDER BY chain_seq`, tenant)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("audit: read chain: %w", err)
	}
	defer rows.Close()

	res := VerifyResult{OK: true}
	prev := Genesis(tenant)
	var expected int64 = 1
	fail := func(seq int64, why string) {
		if res.OK {
			res.OK, res.FirstBadSeq, res.Reason = false, seq, why
		}
	}

	for rows.Next() {
		r, err := scan(rows, tenant)
		if err != nil {
			return VerifyResult{}, err
		}
		res.Checked++
		switch {
		case r.ChainSeq != expected:
			fail(expected, fmt.Sprintf("sequence gap: expected %d, found %d", expected, r.ChainSeq))
			expected = r.ChainSeq
		case r.PrevHash != prev:
			fail(r.ChainSeq, "previous hash does not match the preceding record")
		default:
			if want, herr := r.computeHash(); herr != nil || want != r.RecordHash {
				fail(r.ChainSeq, "record hash does not match its content")
			}
		}
		prev = r.RecordHash
		expected = r.ChainSeq + 1
		res.Head = Anchor{TenantID: tenant, Seq: r.ChainSeq, Hash: r.RecordHash}
	}
	if err := rows.Err(); err != nil {
		return VerifyResult{}, fmt.Errorf("audit: read chain: %w", err)
	}
	rows.Close()

	var headSeq int64
	var headHash string
	err = tx.QueryRow(ctx, `SELECT last_seq, last_hash FROM audit_head($1)`, tenant).Scan(&headSeq, &headHash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if res.Checked > 0 {
			fail(1, "records exist but the chain has no head")
		}
	case err != nil:
		return VerifyResult{}, fmt.Errorf("audit: read head: %w", err)
	case headSeq != res.Head.Seq || (res.Checked > 0 && headHash != res.Head.Hash):
		fail(res.Head.Seq+1, fmt.Sprintf("chain ends at %d but the head says %d: records are missing", res.Head.Seq, headSeq))
	}
	return res, nil
}

// Filter narrows List.
type Filter struct {
	SubjectType string
	SubjectID   string
	ActorID     uuid.UUID
	Action      string
	// SystemOnly limits the result to system and service actors.
	SystemOnly bool
}

// List returns records in chain order, oldest first, after afterSeq.
func List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, f Filter, afterSeq int64, limit int) ([]Record, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("audit: limit must be between 1 and 500")
	}
	if err := requireTenantTx(ctx, tx, tenant); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT `+columns+` FROM audit_events
		WHERE tenant_id = $1 AND chain_seq > $2
		  AND ($3 = '' OR subject_type = $3) AND ($4 = '' OR subject_id = $4)
		  AND ($5::uuid IS NULL OR actor_id = $5) AND ($6 = '' OR action = $6)
		  AND (NOT $7 OR actor_type IN ('system', 'service'))
		ORDER BY chain_seq LIMIT $8`,
		tenant, afterSeq, f.SubjectType, f.SubjectID, nilIfZero(f.ActorID), f.Action, f.SystemOnly, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		r, err := scan(rows, tenant)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nilIfZero(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// CountSince returns how many records of action the actor has written at or
// after since. Rate limits use it, so the limit is as durable as the audit
// trail and shared by every API instance.
func CountSince(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, action string, since time.Time) (int, error) {
	if err := requireTenantTx(ctx, tx, tenant); err != nil {
		return 0, err
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE tenant_id = $1 AND actor_id = $2 AND action = $3 AND occurred_at >= $4`,
		tenant, actor, action, since.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("audit: count: %w", err)
	}
	return n, nil
}
