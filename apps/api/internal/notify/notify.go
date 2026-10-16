// Package notify decides what to tell customers and operators, and records it.
//
// Customer messages are two-stage. The first says a problem was detected and is
// being checked. Only the second, sent after the outcome has been independently
// verified, says funds were returned. Nothing here delivers anything: rows are
// recorded and labelled simulated for the notification mock and the screens.
package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
)

// Audiences and stages.
const (
	Customer   = "customer"
	Operations = "operations"

	StageDetected = "detected"
	StageVerified = "verified"
	StageAlert    = "alert"
)

// Template ids.
const (
	TplCustomerDetected    = "customer.detected.v1"
	TplCustomerVerified    = "customer.reversal_verified.v1"
	TplOpsExceptionOpened  = "ops.exception_opened.v1"
	TplOpsAwaitingApproval = "ops.reversal_awaiting_approval.v1"
	TplOpsReversalVerified = "ops.reversal_verified.v1"
	TplOpsReversalFailed   = "ops.reversal_failed.v1"
	TplOpsHeld             = "ops.reversal_held.v1"
	TplOpsApprovalExpired  = "ops.approval_expired.v1"
	TplCustomerReceived    = "customer.report_received.v1"
	TplOpsCustomerReport   = "ops.customer_report.v1"
)

// Money formats minor units for people, such as "NGN 18,500.00".
func Money(minor int64, currency string) string {
	major, frac := minor/100, minor%100
	s := fmt.Sprintf("%d", major)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return fmt.Sprintf("%s %s.%02d", currency, strings.Join(parts, ","), frac)
}

// Message is a rendered notification.
type Message struct {
	Audience string
	Stage    string
	Template string
	Body     string
}

// CustomerDetected is the first customer message. It makes no promise about money.
func CustomerDetected(minor int64, currency, txnRef string) Message {
	return Message{Customer, StageDetected, TplCustomerDetected,
		fmt.Sprintf("We found a problem with your transaction %s of %s. We are checking it now. You do not need to do anything. We will tell you the result.", txnRef, Money(minor, currency))}
}

// CustomerVerified is the second customer message. It is only to be sent after
// the reversal has been verified against the ledger.
func CustomerVerified(minor int64, currency, txnRef string) Message {
	return Message{Customer, StageVerified, TplCustomerVerified,
		fmt.Sprintf("Your transaction %s of %s was reversed. We confirmed the reversal in your account records.", txnRef, Money(minor, currency))}
}

// CustomerReportReceived acknowledges a customer's report. It says only that the report
// arrived and when the bank aims to respond. It makes no statement about money.
func CustomerReportReceived(ref string, target *time.Time) Message {
	when := "We will tell you when we have a target time."
	if target != nil {
		when = "Our target to respond is " + target.In(time.FixedZone("WAT", 3600)).Format("2 Jan 2006 15:04") + " WAT."
	}
	return Message{Customer, StageDetected, TplCustomerReceived,
		fmt.Sprintf("We have received your report (ref %s). %s You can follow it in ReverseIt.", ref, when)}
}

// OpsCustomerReport tells operations a customer report arrived.
func OpsCustomerReport(caseRef, code, dispute string) Message {
	return Message{Operations, StageAlert, TplOpsCustomerReport,
		fmt.Sprintf("Customer report %s opened as %s (%s).", caseRef, code, dispute)}
}

// OpsExceptionOpened tells operations a case exists.
func OpsExceptionOpened(caseRef, code, txnRef string, minor int64, currency string) Message {
	return Message{Operations, StageAlert, TplOpsExceptionOpened,
		fmt.Sprintf("Exception %s opened as %s for transaction %s (%s).", caseRef, code, txnRef, Money(minor, currency))}
}

// OpsAwaitingApproval asks for a decision.
func OpsAwaitingApproval(caseRef, reason string) Message {
	return Message{Operations, StageAlert, TplOpsAwaitingApproval,
		fmt.Sprintf("Case %s needs an approval before any reversal. Reason: %s.", caseRef, reason)}
}

// OpsApprovalExpired says an approval lapsed before the reversal was submitted.
func OpsApprovalExpired(caseRef, reversalID, why string) Message {
	return Message{Operations, StageAlert, TplOpsApprovalExpired,
		fmt.Sprintf("Case %s: the approval for reversal %s is no longer valid (%s). Nothing was sent. A fresh approval is needed.", caseRef, reversalID, why)}
}

// OpsHeld says a reversal was blocked by the safety gate.
func OpsHeld(caseRef, reason string) Message {
	return Message{Operations, StageAlert, TplOpsHeld,
		fmt.Sprintf("Case %s: reversal blocked. %s. No money was moved.", caseRef, reason)}
}

// OpsReversalVerified reports a verified reversal.
func OpsReversalVerified(caseRef, reversalID string) Message {
	return Message{Operations, StageAlert, TplOpsReversalVerified,
		fmt.Sprintf("Case %s: reversal %s was verified against the ledger.", caseRef, reversalID)}
}

// OpsReversalFailed reports a failed or unverified reversal.
func OpsReversalFailed(caseRef, reversalID, why string) Message {
	return Message{Operations, StageAlert, TplOpsReversalFailed,
		fmt.Sprintf("Case %s: reversal %s did not complete. %s.", caseRef, reversalID, why)}
}

// Record stores m for a case. The dedupe key makes it safe to call twice: the
// second call records nothing, emits nothing and reports created=false. A new
// notification also emits notification.recorded in the same transaction. The
// event names the notification but never carries its text.
func Record(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, m Message, dedupeKey string) (created bool, err error) {
	if dedupeKey == "" {
		return false, fmt.Errorf("notify: a dedupe key is required")
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO notifications (tenant_id, case_id, audience, stage, template_id, body, dedupe_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (tenant_id, dedupe_key) DO NOTHING RETURNING id`,
		tenant, caseID, m.Audience, m.Stage, m.Template, m.Body, dedupeKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("notify: record: %w", err)
	}
	if _, err := events.Emit(ctx, tx, tenant, events.Event{Type: events.NotificationRecorded, AggregateID: caseID.String(), Key: id.String(),
		Fields: map[string]any{"notification_id": id.String(), "case_id": caseID.String(), "audience": m.Audience, "stage": m.Stage, "template_id": m.Template}}); err != nil {
		return false, err
	}
	return true, nil
}

// Stored is a recorded notification.
type Stored struct {
	ID        uuid.UUID `json:"id"`
	Audience  string    `json:"audience"`
	Stage     string    `json:"stage"`
	Template  string    `json:"template_id"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	Simulated bool      `json:"simulated"`
	CreatedAt string    `json:"created_at"`
}

// ForCase lists a case's notifications, oldest first, for one audience (or all
// when audience is empty). Messages with the same timestamp are put in the order
// the customer would read them (detected before verified), so the order is
// always the same.
func ForCase(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, audience string) ([]Stored, error) {
	rows, err := tx.Query(ctx, `SELECT id, audience, stage, template_id, body, status, simulated, to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		FROM notifications WHERE tenant_id = $1 AND case_id = $2 AND ($3 = '' OR audience = $3) ORDER BY created_at, CASE stage WHEN 'detected' THEN 0 WHEN 'verified' THEN 2 ELSE 1 END, id`, tenant, caseID, audience)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stored{}
	for rows.Next() {
		var s Stored
		if err := rows.Scan(&s.ID, &s.Audience, &s.Stage, &s.Template, &s.Body, &s.Status, &s.Simulated, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
