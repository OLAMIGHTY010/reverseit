package identity

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// AuditLoginObserver writes each login attempt to the tenant's audit chain.
type AuditLoginObserver struct {
	Pool     *db.Pool
	Appender *audit.Appender
	Logger   *slog.Logger
}

var _ LoginObserver = (*AuditLoginObserver)(nil)

// Observe records e. Failed attempts by a known user are attributed to that
// user. Attempts by an unknown email are attributed to the system, with the
// email digest in the details.
func (o *AuditLoginObserver) Observe(ctx context.Context, e LoginEvent) error {
	ev := audit.Event{
		Action:      "auth.login_" + string(e.Outcome),
		SubjectType: "user",
		Details:     map[string]any{"email_digest": e.EmailDigest, "outcome": string(e.Outcome)},
	}
	if e.UserID != uuid.Nil {
		ev.Actor, ev.ActorID, ev.SubjectID = audit.ActorUser, e.UserID, e.UserID.String()
	} else {
		ev.Actor = audit.ActorSystem
	}
	err := o.Pool.WithTenant(ctx, e.TenantID, func(tx pgx.Tx) error {
		_, err := o.Appender.Append(ctx, tx, e.TenantID, ev)
		return err
	})
	if err != nil && o.Logger != nil {
		o.Logger.ErrorContext(ctx, "login_audit_failed", slog.String("outcome", string(e.Outcome)), slog.Any("error", err))
	}
	return err
}
