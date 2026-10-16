package reconciliation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

var (
	ErrDuplicateFile = errors.New("duplicate settlement file for this provider and period")
	ErrBreakNotFound = errors.New("reconciliation break not found")
)

// Service provides batch reconciliation and settlement file processing.
type Service struct {
	Pool  *db.Pool
	Cases *cases.Service
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// UploadInput is the parameter payload for uploading and reconciling a settlement file.
type UploadInput struct {
	Provider       string
	Kind           string
	PeriodStart    string
	PeriodEnd      string
	MappingVersion string
	Filename       string
	Content        []byte
	Tolerance      ToleranceConfig
}

// UploadAndReconcile processes an inbound settlement file, stores raw records, detects breaks, and returns a summary.
func (s *Service) UploadAndReconcile(ctx context.Context, tenant uuid.UUID, in UploadInput) (*ReconSummary, error) {
	rows, hash, err := ParseSettlementFile(bytes.NewReader(in.Content), in.Provider)
	if err != nil {
		return nil, err
	}

	var summary ReconSummary
	fileID := uuid.New()
	now := s.now()

	matcher := &Matcher{Cases: s.Cases, Now: s.Now}

	err = s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Check for duplicate
		var existingID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM settlement_files WHERE tenant_id = $1 AND provider = $2 AND period_start = $3 AND period_end = $4 AND sha256 = $5`,
			tenant, in.Provider, in.PeriodStart, in.PeriodEnd, hash).Scan(&existingID)
		if err == nil {
			return ErrDuplicateFile
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Insert settlement file record
		_, err = tx.Exec(ctx, `INSERT INTO settlement_files (tenant_id, id, provider, kind, period_start, period_end, mapping_version, sha256, filename, record_count, raw_content, uploaded_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			tenant, fileID, in.Provider, in.Kind, in.PeriodStart, in.PeriodEnd, in.MappingVersion, hash, in.Filename, len(rows), string(in.Content), now)
		if err != nil {
			return fmt.Errorf("inserting settlement file: %w", err)
		}

		// Batch insert records
		for _, r := range rows {
			recID := uuid.New()
			_, err = tx.Exec(ctx, `INSERT INTO settlement_records (tenant_id, id, file_id, provider, instruction_ref, session_id, amount_minor, currency, settled_on, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				tenant, recID, fileID, r.Provider, r.InstructionRef, r.SessionID, r.AmountMinor, r.Currency, r.SettledOn, r.Status)
			if err != nil {
				return fmt.Errorf("inserting settlement record %s: %w", r.InstructionRef, err)
			}
		}

		// Run reconciliation match engine
		sessionCode := fmt.Sprintf("%s-%s", in.Provider, in.PeriodStart)
		channel := in.Provider
		if in.Provider == "NIBSS" {
			channel = "NIP"
		}
		matchRes, err := matcher.Match(ctx, tx, tenant, &fileID, sessionCode, channel, rows, in.Tolerance)
		if err != nil {
			return fmt.Errorf("matching settlement records: %w", err)
		}

		// Persist detected breaks
		casesOpened := 0
		var netDiscrepancy int64
		for _, b := range matchRes.Breaks {
			netDiscrepancy += b.AmountDiffMinor
			if b.CaseID != nil {
				casesOpened++
			}
			_, err = tx.Exec(ctx, `INSERT INTO reconciliation_breaks (tenant_id, id, file_id, session_code, channel, break_type, break_label, severity, status, instruction_ref, session_id, ledger_amount_minor, switch_amount_minor, amount_diff_minor, time_diff_seconds, case_id, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
				tenant, b.ID, b.FileID, b.SessionCode, b.Channel, b.BreakType, b.BreakLabel, b.Severity, b.Status, b.InstructionRef, b.SessionID, b.LedgerAmountMinor, b.SwitchAmountMinor, b.AmountDiffMinor, b.TimeDiffSeconds, b.CaseID, b.CreatedAt, b.UpdatedAt)
			if err != nil {
				return fmt.Errorf("inserting break for %s: %w", b.InstructionRef, err)
			}
		}

		// Update file counts
		_, err = tx.Exec(ctx, `UPDATE settlement_files SET matched_count = $1, break_count = $2 WHERE tenant_id = $3 AND id = $4`,
			matchRes.MatchedCount, len(matchRes.Breaks), tenant, fileID)
		if err != nil {
			return err
		}

		summary = ReconSummary{
			FileID:            fileID,
			SHA256:            hash,
			TotalRecords:      len(rows),
			MatchedCount:      matchRes.MatchedCount,
			ToleranceCount:    matchRes.ToleranceCount,
			BreakCount:        len(matchRes.Breaks),
			CasesOpenedCount:  casesOpened,
			NetDiscrepancyAmt: netDiscrepancy,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// ListBreaks returns breaks matching the specified filters.
func (s *Service) ListBreaks(ctx context.Context, tenant uuid.UUID, status, breakType string, limit int) ([]Break, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var out []Break
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		query := `SELECT b.id, b.file_id, b.session_code, b.channel, b.break_type, b.break_label, b.severity, b.status,
			b.instruction_ref, b.session_id, b.ledger_amount_minor, b.switch_amount_minor, b.amount_diff_minor, b.time_diff_seconds,
			b.case_id, COALESCE(c.case_ref, ''), b.maker_notes, b.created_at, b.updated_at
			FROM reconciliation_breaks b
			LEFT JOIN cases c ON c.tenant_id = b.tenant_id AND c.id = b.case_id
			WHERE b.tenant_id = $1`
		args := []any{tenant}
		argIdx := 2

		if status != "" && status != "ALL" {
			query += fmt.Sprintf(" AND b.status = $%d", argIdx)
			args = append(args, status)
			argIdx++
		}
		if breakType != "" && breakType != "ALL" {
			query += fmt.Sprintf(" AND b.break_type = $%d", argIdx)
			args = append(args, breakType)
			argIdx++
		}
		query += fmt.Sprintf(" ORDER BY b.created_at DESC LIMIT $%d", argIdx)
		args = append(args, limit)

		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b Break
			var cRef string
			if err := rows.Scan(&b.ID, &b.FileID, &b.SessionCode, &b.Channel, &b.BreakType, &b.BreakLabel, &b.Severity, &b.Status,
				&b.InstructionRef, &b.SessionID, &b.LedgerAmountMinor, &b.SwitchAmountMinor, &b.AmountDiffMinor, &b.TimeDiffSeconds,
				&b.CaseID, &cRef, &b.MakerNotes, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			b.CaseRef = cRef
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// PairBreak manually marks a reconciliation break as paired with maker notes and approval signoff.
func (s *Service) PairBreak(ctx context.Context, tenant, breakID uuid.UUID, actor, notes string) (*Break, error) {
	var out Break
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		signoff, _ := json.Marshal(map[string]any{
			"approved_by": actor,
			"approved_at": s.now().Format(time.RFC3339),
			"status":      "APPROVED",
		})
		res, err := tx.Exec(ctx, `UPDATE reconciliation_breaks SET status = $1, maker_notes = $2, checker_signoff = $3, updated_at = $4
			WHERE tenant_id = $5 AND id = $6`,
			StatusManuallyPaired, notes, signoff, s.now(), tenant, breakID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return ErrBreakNotFound
		}

		err = tx.QueryRow(ctx, `SELECT id, file_id, session_code, channel, break_type, break_label, severity, status, instruction_ref,
			session_id, ledger_amount_minor, switch_amount_minor, amount_diff_minor, time_diff_seconds, case_id, maker_notes, created_at, updated_at
			FROM reconciliation_breaks WHERE tenant_id = $1 AND id = $2`, tenant, breakID).
			Scan(&out.ID, &out.FileID, &out.SessionCode, &out.Channel, &out.BreakType, &out.BreakLabel, &out.Severity, &out.Status,
				&out.InstructionRef, &out.SessionID, &out.LedgerAmountMinor, &out.SwitchAmountMinor, &out.AmountDiffMinor, &out.TimeDiffSeconds,
				&out.CaseID, &out.MakerNotes, &out.CreatedAt, &out.UpdatedAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
