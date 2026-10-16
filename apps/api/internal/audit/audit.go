// Package audit is the append-only, hash-chained decision log.
//
// Every sensitive decision is written in the same transaction as the change it
// describes, so the log and the data cannot disagree. Each tenant has its own
// chain. A record's hash covers its content and the previous record's hash, so
// changing, removing or reordering any row breaks every hash after it.
//
// PostgreSQL enforces append-only (privileges, triggers) and enforces that a new
// row continues the chain. Verify recomputes everything from the rows alone.
package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActorType says who acted.
type ActorType string

const (
	ActorUser    ActorType = "user"
	ActorService ActorType = "service"
	ActorSystem  ActorType = "system"
)

var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_.]{1,80}$`)

// forbiddenDetailKeys must never appear in details: audit rows are readable by
// auditors and exported, and must not become a place secrets leak into.
var forbiddenDetailKeys = []string{"password", "secret", "token", "authorization", "private_key", "api_key", "pan", "cvv"}

// Event is one decision to record.
type Event struct {
	Actor        ActorType
	ActorID      uuid.UUID // zero for the system
	Action       string    // dotted, lower case, for example "reversal.approved"
	SubjectType  string
	SubjectID    string
	ReasonCode   string
	EvidenceRefs []string
	BeforeHash   string
	AfterHash    string
	// PolicyVersion is the rule, policy or model version that applied.
	PolicyVersion string
	IP            string
	Device        string
	// Sensitive marks a read or action on sensitive data, such as an unmask.
	Sensitive bool
	// Details holds structured context. No secrets, and no raw account numbers.
	Details map[string]any
	// OccurredAt defaults to the appender's clock. Stored to microseconds.
	OccurredAt time.Time
}

// Record is a stored event with its chain fields.
type Record struct {
	Event
	TenantID   uuid.UUID
	ChainSeq   int64
	ID         uuid.UUID
	PrevHash   string
	RecordHash string
}

// Appender writes events.
type Appender struct {
	// Now is the time source. Defaults to time.Now. Use one trusted clock.
	Now func() time.Time
}

// Genesis returns the hash that precedes the first record of a tenant.
func Genesis(tenant uuid.UUID) string {
	sum := sha256.Sum256([]byte("SMARTREVERSAL-AUDIT-GENESIS:" + tenant.String()))
	return hex.EncodeToString(sum[:])
}

func (e Event) validate() error {
	switch e.Actor {
	case ActorUser, ActorService:
		if e.ActorID == uuid.Nil {
			return errors.New("audit: actor id is required for user and service actors")
		}
	case ActorSystem:
	default:
		return errors.New("audit: unknown actor type")
	}
	if !actionPattern.MatchString(e.Action) {
		return errors.New("audit: action must be lower case words separated by dots or underscores")
	}
	for k := range e.Details {
		lk := strings.ToLower(k)
		for _, bad := range forbiddenDetailKeys {
			if strings.Contains(lk, bad) {
				return fmt.Errorf("audit: details key %q looks like a secret", k)
			}
		}
	}
	return nil
}

// Append writes e to the tenant's chain using tx, which must be a tenant
// transaction (db.WithTenant). The write commits or rolls back with the caller's
// change. It locks the tenant's chain until then, so concurrent writers for one
// tenant are serialised.
func (a *Appender) Append(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, e Event) (Record, error) {
	if tenant == uuid.Nil {
		return Record{}, errors.New("audit: tenant id is required")
	}
	if err := e.validate(); err != nil {
		return Record{}, err
	}
	if err := requireTenantTx(ctx, tx, tenant); err != nil {
		return Record{}, err
	}

	now := time.Now
	if a != nil && a.Now != nil {
		now = a.Now
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = now()
	}
	e.OccurredAt = e.OccurredAt.UTC().Truncate(time.Microsecond) // PostgreSQL stores microseconds
	if e.EvidenceRefs == nil {
		e.EvidenceRefs = []string{}
	}
	if e.Details == nil {
		e.Details = map[string]any{}
	}

	var lastSeq int64
	var lastHash string
	if err := tx.QueryRow(ctx, `SELECT last_seq, last_hash FROM audit_lock_head($1, $2)`, tenant, Genesis(tenant)).
		Scan(&lastSeq, &lastHash); err != nil {
		return Record{}, fmt.Errorf("audit: lock chain: %w", err)
	}

	r := Record{Event: e, TenantID: tenant, ChainSeq: lastSeq + 1, ID: uuid.New(), PrevHash: lastHash}
	hash, err := r.computeHash()
	if err != nil {
		return Record{}, err
	}
	r.RecordHash = hash

	evidence, _ := json.Marshal(e.EvidenceRefs)
	details, err := canonicalJSON(e.Details)
	if err != nil {
		return Record{}, fmt.Errorf("audit: details: %w", err)
	}
	var actorID any
	if e.ActorID != uuid.Nil {
		actorID = e.ActorID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (tenant_id, chain_seq, id, occurred_at, actor_type, actor_id, action,
			subject_type, subject_id, reason_code, evidence_refs, before_hash, after_hash, policy_version,
			ip, device, sensitive_access, details, prev_hash, record_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		r.TenantID, r.ChainSeq, r.ID, e.OccurredAt, string(e.Actor), actorID, e.Action,
		e.SubjectType, e.SubjectID, e.ReasonCode, evidence, e.BeforeHash, e.AfterHash, e.PolicyVersion,
		e.IP, e.Device, e.Sensitive, details, r.PrevHash, r.RecordHash)
	if err != nil {
		return Record{}, fmt.Errorf("audit: insert: %w", err)
	}
	return r, nil
}

// hashInput is the exact structure that is hashed. Field order is fixed by the
// struct, and map keys inside Details are sorted by encoding/json.
type hashInput struct {
	TenantID      string   `json:"tenant_id"`
	ChainSeq      int64    `json:"chain_seq"`
	ID            string   `json:"id"`
	OccurredAt    string   `json:"occurred_at"`
	ActorType     string   `json:"actor_type"`
	ActorID       string   `json:"actor_id"`
	Action        string   `json:"action"`
	SubjectType   string   `json:"subject_type"`
	SubjectID     string   `json:"subject_id"`
	ReasonCode    string   `json:"reason_code"`
	EvidenceRefs  []string `json:"evidence_refs"`
	BeforeHash    string   `json:"before_hash"`
	AfterHash     string   `json:"after_hash"`
	PolicyVersion string   `json:"policy_version"`
	IP            string   `json:"ip"`
	Device        string   `json:"device"`
	Sensitive     bool     `json:"sensitive_access"`
	Details       any      `json:"details"`
	PrevHash      string   `json:"prev_hash"`
}

func (r Record) computeHash() (string, error) {
	actor := ""
	if r.ActorID != uuid.Nil {
		actor = r.ActorID.String()
	}
	// Details are normalised through JSON so a value read back from jsonb hashes
	// the same as the value that was written.
	details, err := normalise(r.Details)
	if err != nil {
		return "", err
	}
	refs := r.EvidenceRefs
	if refs == nil {
		refs = []string{}
	}
	in := hashInput{
		TenantID: r.TenantID.String(), ChainSeq: r.ChainSeq, ID: r.ID.String(),
		OccurredAt: r.OccurredAt.UTC().Format(time.RFC3339Nano),
		ActorType:  string(r.Actor), ActorID: actor, Action: r.Action,
		SubjectType: r.SubjectType, SubjectID: r.SubjectID, ReasonCode: r.ReasonCode,
		EvidenceRefs: refs, BeforeHash: r.BeforeHash, AfterHash: r.AfterHash,
		PolicyVersion: r.PolicyVersion, IP: r.IP, Device: r.Device, Sensitive: r.Sensitive,
		Details: details, PrevHash: r.PrevHash,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(in); err != nil {
		return "", fmt.Errorf("audit: hash input: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

// normalise round-trips v through JSON with numbers kept as written, so maps
// compare and hash the same before and after PostgreSQL stores them.
func normalise(v map[string]any) (any, error) {
	if v == nil {
		v = map[string]any{}
	}
	raw, err := canonicalJSON(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
