// Package authz decides what a principal may do. The permission matrix is data
// loaded from the database (migration 0005). This package adds the attribute
// rules that data alone cannot express: amount limits, ownership, reasons,
// second approvers and the MFA requirement for privileged actions.
//
// It fails closed. An unknown role, an unknown capability, a missing limit or a
// missing attribute is a denial.
package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// Errors returned by Check. Handlers map ErrForbidden to 403. ErrMFARequired is
// a separate error so a client can be told to step up, and it is never returned
// to someone who lacked the capability in the first place.
var (
	ErrForbidden   = errors.New("authz: forbidden")
	ErrMFARequired = errors.New("authz: multi-factor authentication required")
)

// Grant is what a role may do with a capability.
type Grant struct {
	Role       string
	Capability string
	// Scope narrows the grant, for example "masked" or "own". Handlers must
	// honour it when shaping output.
	Scope string
}

// Attrs are the facts about the specific request being checked. Unset fields
// mean "not provided", and a rule that needs one denies.
type Attrs struct {
	// AmountMinor is the value of the item being approved, in minor units.
	AmountMinor *int64
	// LimitMinor is the caller's limit for within_limit and higher_limit grants,
	// read by the caller from versioned configuration. Nil means no limit is
	// configured, which denies.
	LimitMinor *int64
	// OwnerID is who owns the item, for the "own" scope. Scopes such as
	// own_customer narrow what a handler returns and are not checked here.
	OwnerID uuid.UUID
	// Reason is the free-text justification, required by reason-gated scopes.
	Reason string
	// SecondApprover is a different principal who has approved, required by
	// dual-control scopes.
	SecondApprover uuid.UUID
}

// Policy is an immutable snapshot of the matrix.
type Policy struct {
	grants map[string]map[string]string // role -> capability -> scope
	mfa    map[string]bool              // capability -> requires MFA
	roles  map[string]bool
}

// NewPolicy builds a Policy. It is used by Load and by tests.
func NewPolicy(grants []Grant, mfa map[string]bool, roles []string) *Policy {
	p := &Policy{grants: map[string]map[string]string{}, mfa: map[string]bool{}, roles: map[string]bool{}}
	for _, r := range roles {
		p.roles[r] = true
	}
	for c, m := range mfa {
		p.mfa[c] = m
	}
	for _, g := range grants {
		if p.grants[g.Role] == nil {
			p.grants[g.Role] = map[string]string{}
		}
		p.grants[g.Role][g.Capability] = g.Scope
		if _, known := p.mfa[g.Capability]; !known {
			p.mfa[g.Capability] = false
		}
	}
	return p
}

// Load reads the matrix. It uses the pool's own role because the matrix is
// reference data that applies to every tenant.
func Load(ctx context.Context, pool *db.Pool) (*Policy, error) {
	rows, err := pool.Query(ctx, `SELECT role, capability, scope FROM permission_grants`)
	if err != nil {
		return nil, fmt.Errorf("authz: load grants: %w", err)
	}
	var grants []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.Role, &g.Capability, &g.Scope); err != nil {
			rows.Close()
			return nil, err
		}
		grants = append(grants, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mfa := map[string]bool{}
	crows, err := pool.Query(ctx, `SELECT capability, requires_mfa FROM capabilities`)
	if err != nil {
		return nil, fmt.Errorf("authz: load capabilities: %w", err)
	}
	for crows.Next() {
		var c string
		var m bool
		if err := crows.Scan(&c, &m); err != nil {
			crows.Close()
			return nil, err
		}
		mfa[c] = m
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}

	var roles []string
	rrows, err := pool.Query(ctx, `SELECT role FROM roles`)
	if err != nil {
		return nil, fmt.Errorf("authz: load roles: %w", err)
	}
	for rrows.Next() {
		var r string
		if err := rrows.Scan(&r); err != nil {
			rrows.Close()
			return nil, err
		}
		roles = append(roles, r)
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return nil, errors.New("authz: the permission matrix is empty")
	}
	return NewPolicy(grants, mfa, roles), nil
}

// Scope of a capability by breadth, widest last. When a principal holds several
// roles that grant the same capability, the widest scope wins.
var breadth = map[string]int{
	"notes_only": 1, "own_customer": 1, "own": 1, "metadata": 1, "aggregate": 1,
	"masked": 2, "reason_gated": 2, "propose": 2, "draft": 2, "review": 2,
	"team": 3, "within_limit": 3, "approve": 3, "controlled": 3, "dual_control": 3, "override_dual": 3, "system": 3,
	"higher_limit": 4, "all_ops": 4, "all": 5, "full": 6,
}

// Check reports whether p may use capability given a. On success it returns the
// grant, whose Scope the handler must apply. A capability marked as needing MFA
// is refused with ErrMFARequired unless the token carries it.
func (pol *Policy) Check(p *identity.Principal, capability string, a Attrs) (Grant, error) {
	g, err := pol.GrantFor(p, capability, a)
	if err != nil {
		return Grant{}, err
	}
	if pol.mfa[capability] && !p.MFA {
		return Grant{}, ErrMFARequired
	}
	return g, nil
}

// GrantFor is Check without the MFA rule. Use it for the read-only and
// preparatory uses of a capability that is privileged only when it is exercised,
// such as reading configuration or proposing a change, where the approval is
// what needs MFA. Everything else about the check is the same.
func (pol *Policy) GrantFor(p *identity.Principal, capability string, a Attrs) (Grant, error) {
	if pol == nil || p == nil {
		return Grant{}, ErrForbidden
	}
	if _, known := pol.mfa[capability]; !known {
		return Grant{}, ErrForbidden
	}

	var best *Grant
	for _, role := range p.Roles {
		if !pol.roles[role] {
			continue // a role the matrix does not know grants nothing
		}
		scope, ok := pol.grants[role][capability]
		if !ok {
			continue
		}
		if !pol.scopeSatisfied(p, scope, a) {
			continue
		}
		if best == nil || breadth[scope] > breadth[best.Scope] {
			g := Grant{Role: role, Capability: capability, Scope: scope}
			best = &g
		}
	}
	if best == nil {
		return Grant{}, ErrForbidden
	}
	return *best, nil
}

// scopeSatisfied applies the attribute rule that goes with a scope.
func (pol *Policy) scopeSatisfied(p *identity.Principal, scope string, a Attrs) bool {
	switch scope {
	case "within_limit", "higher_limit":
		return a.AmountMinor != nil && a.LimitMinor != nil && *a.AmountMinor >= 0 && *a.AmountMinor <= *a.LimitMinor
	case "own":
		return a.OwnerID != uuid.Nil && a.OwnerID == p.UserID
	case "reason_gated":
		return len(trimSpace(a.Reason)) >= 10
	case "override_dual", "dual_control":
		return len(trimSpace(a.Reason)) >= 10 &&
			a.SecondApprover != uuid.Nil && a.SecondApprover != p.UserID
	default:
		return true
	}
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
