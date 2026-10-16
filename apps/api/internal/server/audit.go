package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/creport"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/incidents"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reconciliation"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Services are the collaborators that protected routes share.
type Services struct {
	Pool   *db.Pool
	Policy *authz.Policy
	Audit  *audit.Appender
	// Vault enables POST /v1/accounts/unmask. Nil leaves the route out.
	Vault *datashield.Vault
	// Config enables the /v1/config and /v1/calendar routes. Nil leaves them out.
	Config *configsvc.Service
	// KillSwitch enables the /v1/killswitch routes. Nil leaves them out.
	KillSwitch *killswitch.Service
	// Txn enables transaction ingestion (needs ServiceVerifier on Options) and the
	// transaction read routes. Nil leaves them out.
	Txn *txn.Service
	// Feed enables POST /v1/feeds/{mapping}/files (needs ServiceVerifier on Options).
	Feed *feed.Importer
	// CustomerLogin enables the ReverseIt customer routes under /v1/customer. Nil
	// leaves them out entirely.
	CustomerLogin *customer.Login
	// Alerts and Incidents enable /v1/alerts and /v1/incidents. Nil leaves them out.
	Alerts    *alerts.Service
	Incidents *incidents.Service
	// Reports enables the customer report routes (needs CustomerLogin).
	Reports *creport.Service
	// Rules enables the /v1/rules routes. Nil leaves them out.
	Rules *rules.Engine
	// Redis, when set, lets GET /v1/pipeline report stream backlog. Nil is allowed:
	// the report then covers the outbox only.
	Redis *redis.Client
	// Correlate enables POST /v1/observations and the evidence routes (needs Txn).
	Correlate *correlate.Matcher
	// Cases enables the exception queue and case routes. Nil leaves them out.
	Cases *cases.Service
	// Reversal enables the simulated reversal and approval routes (needs Cases).
	Reversal *reversal.Service
	// Reconciliation enables settlement file ingestion and break detection.
	Reconciliation *reconciliation.Service
	// Routing enables operator shift assignments and delegations.
	Routing *routing.Service
	// UnmaskLimit reveals are allowed per UnmaskWindow for each caller.
	// Defaults are 10 per 10 minutes.
	UnmaskLimit  int
	UnmaskWindow time.Duration
	// Now is the clock for rate limiting. Defaults to time.Now.
	Now func() time.Time
}

func (s *Services) ready() bool {
	return s != nil && s.Pool != nil && s.Policy != nil && s.Audit != nil
}

// authorize runs the policy check, including the MFA rule, and writes the
// failure response itself.
func (s *Services) authorize(w http.ResponseWriter, r *http.Request, capability string, a authz.Attrs) (identity.Principal, authz.Grant, bool) {
	return s.authorizeWith(w, r, capability, a, true)
}

// authorizeNoMFA is authorize without the MFA rule, for reading and for
// preparing a change whose approval is what needs MFA.
func (s *Services) authorizeNoMFA(w http.ResponseWriter, r *http.Request, capability string, a authz.Attrs) (identity.Principal, authz.Grant, bool) {
	return s.authorizeWith(w, r, capability, a, false)
}

func (s *Services) authorizeWith(w http.ResponseWriter, r *http.Request, capability string, a authz.Attrs, mfa bool) (identity.Principal, authz.Grant, bool) {
	p := identity.FromContext(r.Context())
	if p == nil {
		unauthorized(w, r)
		return identity.Principal{}, authz.Grant{}, false
	}
	check := s.Policy.GrantFor
	if mfa {
		check = s.Policy.Check
	}
	g, err := check(p, capability, a)
	switch {
	case err == nil:
		return *p, g, true
	case errors.Is(err, authz.ErrMFARequired):
		httpx.WriteError(w, r, http.StatusForbidden, "MFA_REQUIRED", "multi-factor authentication is required for this action")
	default:
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "you do not have permission to do this")
	}
	return identity.Principal{}, authz.Grant{}, false
}

type auditRecordJSON struct {
	Seq           int64          `json:"seq"`
	ID            string         `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ActorType     string         `json:"actor_type"`
	ActorID       string         `json:"actor_id,omitempty"`
	Action        string         `json:"action"`
	SubjectType   string         `json:"subject_type,omitempty"`
	SubjectID     string         `json:"subject_id,omitempty"`
	ReasonCode    string         `json:"reason_code,omitempty"`
	EvidenceRefs  []string       `json:"evidence_refs"`
	BeforeHash    string         `json:"before_hash,omitempty"`
	AfterHash     string         `json:"after_hash,omitempty"`
	PolicyVersion string         `json:"policy_version,omitempty"`
	Sensitive     bool           `json:"sensitive_access"`
	Details       map[string]any `json:"details"`
	PrevHash      string         `json:"prev_hash"`
	RecordHash    string         `json:"record_hash"`
}

type chainJSON struct {
	Verified bool   `json:"verified"`
	Checked  int64  `json:"checked"`
	BadSeq   int64  `json:"first_bad_seq,omitempty"`
	Reason   string `json:"reason,omitempty"`
	HeadSeq  int64  `json:"head_seq"`
	HeadHash string `json:"head_hash,omitempty"`
}

type auditListResponse struct {
	Items      []auditRecordJSON `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Chain      *chainJSON        `json:"chain,omitempty"`
}

// listAudit serves GET /v1/audit. What the caller sees depends on the scope of
// their view_audit grant. The read is itself recorded in the audit trail.
func (s *Services) listAudit(w http.ResponseWriter, r *http.Request) {
	p0 := identity.FromContext(r.Context())
	if p0 == nil {
		unauthorized(w, r)
		return
	}
	// The own scope is checked against the caller, so offer them as the owner.
	p, grant, ok := s.authorize(w, r, "view_audit", authz.Attrs{OwnerID: p0.UserID})
	if !ok {
		return
	}

	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var after int64
	if page.Cursor != "" {
		after, err = strconv.ParseInt(page.Cursor, 10, 64)
		if err != nil || after < 0 {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor is invalid")
			return
		}
	}

	q := r.URL.Query()
	filter := audit.Filter{SubjectType: q.Get("subject_type"), SubjectID: q.Get("subject_id"), Action: q.Get("action")}
	if v := q.Get("actor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "actor_id must be a UUID")
			return
		}
		filter.ActorID = id
	}
	wantVerify := q.Get("verify") == "true"

	// Narrow by scope. Anything not understood is refused rather than widened.
	switch grant.Scope {
	case "own":
		if filter.ActorID != uuid.Nil && filter.ActorID != p.UserID {
			httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "you may only view your own audit records")
			return
		}
		filter.ActorID = p.UserID
	case "all", "all_ops":
	case "system":
		filter.SystemOnly = true
	default:
		// "team" needs the team model that arrives with case management (B4).
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "this audit scope is not available yet")
		return
	}
	if wantVerify && grant.Scope != "all" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "chain verification needs the all scope")
		return
	}

	var resp auditListResponse
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		recs, err := audit.List(r.Context(), tx, p.TenantID, filter, after, page.Limit+1)
		if err != nil {
			return err
		}
		resp.Items = make([]auditRecordJSON, 0, len(recs))
		for _, rec := range recs {
			resp.Items = append(resp.Items, toJSON(rec))
		}
		if len(resp.Items) > page.Limit {
			resp.Items = resp.Items[:page.Limit]
			resp.NextCursor = httpx.EncodeCursor(strconv.FormatInt(resp.Items[page.Limit-1].Seq, 10))
		}
		if wantVerify {
			v, err := audit.Verify(r.Context(), tx, p.TenantID)
			if err != nil {
				return err
			}
			resp.Chain = &chainJSON{Verified: v.OK, Checked: v.Checked, BadSeq: v.FirstBadSeq, Reason: v.Reason,
				HeadSeq: v.Head.Seq, HeadHash: v.Head.Hash}
		}
		// Reading the audit trail is itself audited, in the same transaction.
		_, err = s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{
			Actor: audit.ActorUser, ActorID: p.UserID, Action: "audit.read", Sensitive: true,
			IP: clientIP(r),
			Details: map[string]any{"scope": grant.Scope, "returned": len(resp.Items), "verified": wantVerify,
				"subject_type": filter.SubjectType, "subject_id": filter.SubjectID},
		})
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func toJSON(r audit.Record) auditRecordJSON {
	out := auditRecordJSON{
		Seq: r.ChainSeq, ID: r.ID.String(), OccurredAt: r.OccurredAt, ActorType: string(r.Actor),
		Action: r.Action, SubjectType: r.SubjectType, SubjectID: r.SubjectID, ReasonCode: r.ReasonCode,
		EvidenceRefs: r.EvidenceRefs, BeforeHash: r.BeforeHash, AfterHash: r.AfterHash,
		PolicyVersion: r.PolicyVersion, Sensitive: r.Sensitive, Details: r.Details,
		PrevHash: r.PrevHash, RecordHash: r.RecordHash,
	}
	if r.ActorID != uuid.Nil {
		out.ActorID = r.ActorID.String()
	}
	if out.EvidenceRefs == nil {
		out.EvidenceRefs = []string{}
	}
	if out.Details == nil {
		out.Details = map[string]any{}
	}
	return out
}

// clientIP returns the connection's peer address. Forwarding headers are not
// trusted: anyone can set them. Put the real client address in once a trusted
// proxy list exists.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}
