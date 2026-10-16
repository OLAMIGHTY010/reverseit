package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const maxEventBody = 64 << 10

type eventResponse struct {
	EventID        string `json:"event_id"`
	TxnID          string `json:"txn_id"`
	LifecycleState string `json:"lifecycle_state"`
	Duplicate      bool   `json:"duplicate"`
}

// ingestEvent serves POST /v1/events. Only a service holding events:write may
// call it. The tenant comes from the service's account, never from the body.
//
// 202 accepted, 200 an exact repeat, 409 the source's reference was reused with
// different content, 422 the event is invalid or contradicts its transaction.
// Invalid events are quarantined for inspection.
func (s *Services) ingestEvent(w http.ResponseWriter, r *http.Request) {
	svc := servauth.FromContext(r.Context())
	if svc == nil {
		unauthorized(w, r)
		return
	}
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}

	if v := r.Header.Get("X-Schema-Version"); v != "" && !supportedSchema(v) {
		httpx.WriteError(w, r, http.StatusBadRequest, "UNSUPPORTED_SCHEMA_VERSION", "X-Schema-Version "+v+" is not supported; this API reads 1.x")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventBody))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "event body is too large")
		return
	}
	var env txn.Envelope
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		s.quarantine(r, svc, "", "malformed JSON: "+err.Error(), httpx.Fingerprint(body))
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "event is not valid JSON for this schema: "+err.Error())
		return
	}
	if env.SourceEventRef != key {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "Idempotency-Key must equal source_event_ref")
		return
	}

	var res txn.Result
	err = s.Pool.WithTenant(r.Context(), svc.TenantID, func(tx pgx.Tx) error {
		if env.InstitutionID != "" {
			var code string
			if err := tx.QueryRow(r.Context(), `SELECT code FROM institutions WHERE id = $1`, svc.TenantID).Scan(&code); err != nil {
				return err
			}
			if code != env.InstitutionID {
				return errInstitutionMismatch
			}
		}
		var err error
		res, err = s.Txn.Ingest(r.Context(), tx, svc.TenantID, env)
		return err
	})
	switch {
	case err == nil:
	case errors.Is(err, txn.ErrInvalid):
		s.quarantineEnvelope(r, svc, env, err.Error())
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
		return
	case errors.Is(err, txn.ErrCoreMismatch):
		s.quarantineEnvelope(r, svc, env, err.Error())
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
		return
	case errors.Is(err, errInstitutionMismatch):
		s.quarantineEnvelope(r, svc, env, err.Error())
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "institution_id does not match the institution these credentials belong to")
		return
	case errors.Is(err, txn.ErrConflict):
		s.dataQualityAlert(r, svc, env)
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "source_event_ref was already used with different content")
		return
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}

	code := http.StatusAccepted
	if res.Outcome == txn.Duplicate {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, eventResponse{EventID: res.EventID.String(), TxnID: res.TxnID.String(),
		LifecycleState: res.Lifecycle, Duplicate: res.Outcome == txn.Duplicate})
}

var errInstitutionMismatch = errors.New("institution_id does not match the institution of the sending service")

// supportedSchema reports whether the schema version header names a 1.x version.
func supportedSchema(v string) bool {
	major, minor, ok := strings.Cut(strings.TrimSpace(v), ".")
	if !ok || major != "1" || minor == "" || len(minor) > 4 {
		return false
	}
	for _, c := range minor {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// dataQualityAlert records that a source reused an event reference for different
// content. It is an audit record naming the source and the reference, written in its
// own transaction so it survives the refusal. It carries no event content.
func (s *Services) dataQualityAlert(r *http.Request, svc *servauth.Principal, env txn.Envelope) {
	_ = s.Pool.WithTenant(r.Context(), svc.TenantID, func(tx pgx.Tx) error {
		_, err := s.Audit.Append(r.Context(), tx, svc.TenantID, audit.Event{
			Actor: audit.ActorService, ActorID: svc.ServiceID, Action: "ingest.data_quality_conflict", SubjectType: "source_event",
			SubjectID: env.SourceSystem + "/" + env.SourceEventRef, Details: map[string]any{"source_system": env.SourceSystem, "source_event_ref": env.SourceEventRef,
				"rejected_payload_hash": env.Hash()},
		})
		return err
	})
}

// quarantine records a rejected event in its own transaction. A failure to
// record it is not allowed to change the answer the caller gets.
func (s *Services) quarantine(r *http.Request, svc *servauth.Principal, source, reason, hash string) {
	_ = s.Pool.WithTenant(r.Context(), svc.TenantID, func(tx pgx.Tx) error {
		return s.Txn.Quarantine(r.Context(), tx, svc.TenantID, source, reason, hash)
	})
}

// quarantineEnvelope is quarantine for an event that decoded, keeping it so it can
// be replayed.
func (s *Services) quarantineEnvelope(r *http.Request, svc *servauth.Principal, env txn.Envelope, reason string) {
	_ = s.Pool.WithTenant(r.Context(), svc.TenantID, func(tx pgx.Tx) error {
		return s.Txn.QuarantineEnvelope(r.Context(), tx, svc.TenantID, env, reason)
	})
}

type transactionJSON struct {
	ID                string    `json:"id"`
	InstructionRef    string    `json:"instruction_ref"`
	Channel           string    `json:"channel"`
	Rail              string    `json:"rail"`
	AmountMinor       string    `json:"amount_minor"`
	Currency          string    `json:"currency"`
	DebitAccountToken string    `json:"debit_account_token"`
	BeneficiaryBank   string    `json:"beneficiary_bank,omitempty"`
	SessionID         string    `json:"session_id,omitempty"`
	LifecycleState    string    `json:"lifecycle_state"`
	DebitStatus       string    `json:"debit_status"`
	CreditStatus      string    `json:"credit_status"`
	SettlementStatus  string    `json:"settlement_status"`
	ReversalStatus    string    `json:"reversal_status"`
	FirstEventAt      time.Time `json:"first_event_at"`
	LastEventAt       time.Time `json:"last_event_at"`
	EventCount        int       `json:"event_count"`
	Simulated         bool      `json:"simulated"`
}

type transactionEventJSON struct {
	ID             string    `json:"id"`
	SourceSystem   string    `json:"source_system"`
	SourceEventRef string    `json:"source_event_ref"`
	EventType      string    `json:"event_type"`
	SourceTS       time.Time `json:"source_ts"`
}

const txnColumns = `id, instruction_ref, channel, rail, amount_minor, currency, debit_account_token, beneficiary_bank, session_id,
	lifecycle_state, debit_status, credit_status, settlement_status, reversal_status, first_event_at, last_event_at, event_count`

func scanTxn(row pgx.Row) (transactionJSON, error) {
	var t transactionJSON
	var amount int64
	if err := row.Scan(&t.ID, &t.InstructionRef, &t.Channel, &t.Rail, &amount, &t.Currency, &t.DebitAccountToken, &t.BeneficiaryBank,
		&t.SessionID, &t.LifecycleState, &t.DebitStatus, &t.CreditStatus, &t.SettlementStatus, &t.ReversalStatus,
		&t.FirstEventAt, &t.LastEventAt, &t.EventCount); err != nil {
		return t, err
	}
	t.AmountMinor = txn.Amount(amount).String()
	t.Simulated = true // Track H: every transaction in this build is synthetic
	t.FirstEventAt, t.LastEventAt = t.FirstEventAt.UTC(), t.LastEventAt.UTC()
	return t, nil
}

// listTransactions serves GET /v1/transactions.
func (s *Services) listTransactions(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" && grant.Scope != "metadata" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not list transactions")
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var cursorTime time.Time
	var cursorID string
	if page.Cursor != "" {
		parts := strings.SplitN(page.Cursor, "|", 2)
		if len(parts) != 2 {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor is invalid")
			return
		}
		if cursorTime, err = time.Parse(time.RFC3339Nano, parts[0]); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor is invalid")
			return
		}
		cursorID = parts[1]
	}
	q := r.URL.Query()
	var items []transactionJSON
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT `+txnColumns+` FROM transactions
			WHERE ($1 = '' OR lifecycle_state = $1) AND ($2 = '' OR rail = $2) AND ($3 = '' OR instruction_ref = $3)
			  AND ($4::timestamptz IS NULL OR (last_event_at, id::text) < ($4, $5))
			ORDER BY last_event_at DESC, id DESC LIMIT $6`,
			q.Get("state"), q.Get("rail"), q.Get("reference"), nilTime(cursorTime), cursorID, page.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTxn(rows)
			if err != nil {
				return err
			}
			items = append(items, t)
		}
		return rows.Err()
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	out := httpx.NewPage(items, page.Limit, func(last transactionJSON) string {
		return last.LastEventAt.Format(time.RFC3339Nano) + "|" + last.ID
	})
	httpx.WriteJSON(w, http.StatusOK, out)
}

func nilTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// getTransaction serves GET /v1/transactions/{id} with its event timeline.
func (s *Services) getTransaction(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" && grant.Scope != "metadata" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not view transactions")
		return
	}
	id := pathParam(r, "id")
	var t transactionJSON
	events := []transactionEventJSON{}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		t, err = scanTxn(tx.QueryRow(r.Context(), `SELECT `+txnColumns+` FROM transactions WHERE id::text = $1`, id))
		if err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT id, source_system, source_event_ref, event_type, source_ts FROM transaction_events
			WHERE txn_id::text = $1 ORDER BY source_ts, id`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e transactionEventJSON
			if err := rows.Scan(&e.ID, &e.SourceSystem, &e.SourceEventRef, &e.EventType, &e.SourceTS); err != nil {
				return err
			}
			e.SourceTS = e.SourceTS.UTC()
			events = append(events, e)
		}
		return rows.Err()
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such transaction")
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"transaction": t, "events": events})
	}
}
