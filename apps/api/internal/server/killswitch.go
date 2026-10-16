package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type frozenJSON struct {
	Scope  string    `json:"scope"`
	Target string    `json:"target"`
	By     uuid.UUID `json:"frozen_by"`
	Reason string    `json:"reason"`
	At     time.Time `json:"frozen_at"`
}

type pendingJSON struct {
	RequestID   int64     `json:"request_id"`
	Scope       string    `json:"scope"`
	Target      string    `json:"target"`
	RequestedBy uuid.UUID `json:"requested_by"`
	Reason      string    `json:"reason"`
	At          time.Time `json:"requested_at"`
}

type killswitchState struct {
	Frozen  []frozenJSON  `json:"frozen"`
	Pending []pendingJSON `json:"pending_unfreeze"`
	// What the freeze is holding back, so screens can say so. Deferred work is
	// detection waiting for an unfreeze; parked reversals are approved but not yet
	// submitted; jobs show whether each scheduled job is running or paused.
	DeferredWork    map[string]int   `json:"deferred_work"`
	ParkedReversals int              `json:"parked_reversals"`
	Jobs            []automation.Job `json:"jobs"`
}

func stateJSON(st killswitch.State) killswitchState {
	out := killswitchState{Frozen: []frozenJSON{}, Pending: []pendingJSON{}, DeferredWork: map[string]int{}, Jobs: []automation.Job{}}
	for _, f := range st.Frozen {
		out.Frozen = append(out.Frozen, frozenJSON{string(f.Target.Scope), f.Target.Value, f.By, f.Reason, f.At})
	}
	for _, p := range st.Pending {
		out.Pending = append(out.Pending, pendingJSON{p.RequestSeq, string(p.Target.Scope), p.Target.Value, p.RequestedBy, p.Reason, p.At})
	}
	return out
}

// getKillSwitch serves GET /v1/killswitch. Every signed-in operator may see the
// switch, because screens must be able to show that automation is frozen.
func (s *Services) getKillSwitch(w http.ResponseWriter, r *http.Request) {
	p := identity.FromContext(r.Context())
	if p == nil {
		unauthorized(w, r)
		return
	}
	var st killswitch.State
	var deferred map[string]int
	var parked int
	var jobs []automation.Job
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		if st, err = s.KillSwitch.State(r.Context(), tx, p.TenantID); err != nil {
			return err
		}
		if deferred, err = automation.Counts(r.Context(), tx, p.TenantID); err != nil {
			return err
		}
		if err = tx.QueryRow(r.Context(), `SELECT count(*)::int FROM reversals WHERE tenant_id = $1 AND parked_at IS NOT NULL`, p.TenantID).Scan(&parked); err != nil {
			return err
		}
		jobs, err = automation.Jobs(r.Context(), tx, p.TenantID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	out := stateJSON(st)
	out.DeferredWork, out.ParkedReversals, out.Jobs = deferred, parked, jobs
	httpx.WriteJSON(w, http.StatusOK, out)
}

type killswitchRequest struct {
	Action    string `json:"action"`
	Scope     string `json:"scope"`
	Target    string `json:"target"`
	Reason    string `json:"reason"`
	RequestID int64  `json:"request_id"`
}

// changeKillSwitch serves POST /v1/killswitch.
//
//	freeze            stops automation at a scope, at once, by one person
//	request_unfreeze  asks to lift a freeze (MFA needed)
//	approve_unfreeze  lifts it, by a different person (MFA needed)
//	reject_unfreeze   keeps it frozen (MFA needed)
//
// The actions are safe to repeat: freezing a frozen scope changes nothing, and a
// request that is already open or already decided is refused.
func (s *Services) changeKillSwitch(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorize(w, r, "trigger_kill_switch", authz.Attrs{})
	if !ok {
		return
	}
	var req killswitchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "request body must be a JSON object with action, reason and, for freeze or request_unfreeze, scope and target")
		return
	}
	// Lifting a freeze is the risky direction, so it needs MFA. Freezing does not:
	// stopping automation must always be quick.
	if req.Action != "freeze" && !p.MFA {
		httpx.WriteError(w, r, http.StatusForbidden, "MFA_REQUIRED", "multi-factor authentication is required to lift a freeze")
		return
	}

	target := killswitch.Target{Scope: killswitch.Scope(req.Scope), Value: req.Target}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		switch req.Action {
		case "freeze":
			_, err := s.KillSwitch.Freeze(r.Context(), tx, p.TenantID, p.UserID, target, req.Reason)
			return err
		case "request_unfreeze":
			_, err := s.KillSwitch.RequestUnfreeze(r.Context(), tx, p.TenantID, p.UserID, target, req.Reason)
			return err
		case "approve_unfreeze", "reject_unfreeze":
			return s.KillSwitch.DecideUnfreeze(r.Context(), tx, p.TenantID, p.UserID, req.RequestID, req.Action == "approve_unfreeze", req.Reason)
		default:
			return errUnknownAction
		}
	})
	switch {
	case err == nil:
	case errors.Is(err, errUnknownAction):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "action must be freeze, request_unfreeze, approve_unfreeze or reject_unfreeze")
		return
	case errors.Is(err, killswitch.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
		return
	case errors.Is(err, killswitch.ErrSameActor):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", "a different person must decide your unfreeze request")
		return
	case errors.Is(err, killswitch.ErrNoSuchRequest):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such open unfreeze request")
		return
	case errors.Is(err, killswitch.ErrNotFrozen), errors.Is(err, killswitch.ErrPending), errors.Is(err, killswitch.ErrAlreadyDecided):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
		return
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}

	s.getKillSwitch(w, r)
}

var errUnknownAction = errors.New("unknown action")
