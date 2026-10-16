package reconciliation

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

type txnMatchRow struct {
	ID           uuid.UUID
	Ref          string
	Channel      string
	Rail         string
	AmountMinor  int64
	Currency     string
	DebitStatus  string
	CreditStatus string
	LastEventAt  time.Time
}

// Matcher evaluates settlement rows against ledger transactions.
type Matcher struct {
	Cases *cases.Service
	Now   func() time.Time
}

func (m *Matcher) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

// MatchResult contains the breakdown of matched rows and detected breaks.
type MatchResult struct {
	MatchedCount   int
	ToleranceCount int
	Breaks         []Break
}

// Match compares a slice of settlement rows against the database transactions for the given period.
func (m *Matcher) Match(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, fileID *uuid.UUID, sessionCode, channel string, rows []SettlementRow, tol ToleranceConfig) (MatchResult, error) {
	// 1. Index settlement rows by instruction ref
	settlementMap := make(map[string]SettlementRow, len(rows))
	for _, r := range rows {
		settlementMap[r.InstructionRef] = r
	}

	// 2. Fetch relevant transactions from DB
	qRows, err := tx.Query(ctx, `SELECT id, instruction_ref, channel, rail, amount_minor, currency, debit_status, credit_status, last_event_at
		FROM transactions WHERE tenant_id = $1`, tenant)
	if err != nil {
		return MatchResult{}, fmt.Errorf("querying transactions for recon: %w", err)
	}
	defer qRows.Close()

	txnMap := make(map[string]txnMatchRow)
	for qRows.Next() {
		var t txnMatchRow
		if err := qRows.Scan(&t.ID, &t.Ref, &t.Channel, &t.Rail, &t.AmountMinor, &t.Currency, &t.DebitStatus, &t.CreditStatus, &t.LastEventAt); err != nil {
			return MatchResult{}, err
		}
		txnMap[t.Ref] = t
	}
	if err := qRows.Err(); err != nil {
		return MatchResult{}, err
	}

	var res MatchResult
	now := m.now()

	// 3. Process every settlement row
	for _, sRow := range rows {
		t, found := txnMap[sRow.InstructionRef]
		if !found {
			// SWITCH_RECORD_NO_LEDGER break
			diff := sRow.AmountMinor
			sev := "HIGH"
			if diff >= 1_000_000 {
				sev = "CRITICAL"
			}
			label := fmt.Sprintf("Switch record without ledger entry: %s (%s %d)", sRow.InstructionRef, sRow.Currency, diff)
			brk := Break{
				ID:                uuid.New(),
				FileID:            fileID,
				SessionCode:       sessionCode,
				Channel:           channel,
				BreakType:         BreakSwitchNoLedger,
				BreakLabel:        label,
				Severity:          sev,
				Status:            StatusUnmatched,
				InstructionRef:    sRow.InstructionRef,
				SessionID:         sRow.SessionID,
				SwitchAmountMinor: &sRow.AmountMinor,
				AmountDiffMinor:   diff,
				TimeDiffSeconds:   0,
				CreatedAt:         now,
				UpdatedAt:         now,
			}

			// Open STL001 case if cases service is available
			if m.Cases != nil {
				cID, cRef, err := m.openCase(ctx, tx, tenant, nil, diff, sRow.Currency, channel, "switch_record_no_ledger", sev, label, brk)
				if err == nil {
					brk.CaseID = cID
					brk.CaseRef = cRef
				}
			}

			res.Breaks = append(res.Breaks, brk)
			continue
		}

		// Transaction exists: compare amounts
		diff := sRow.AmountMinor - t.AmountMinor
		absDiff := int64(math.Abs(float64(diff)))

		if absDiff == 0 {
			// Exact match on amount: check timing cutoff
			timeDiffSec := int(math.Abs(sRow.SettledOn.Sub(t.LastEventAt).Seconds()))
			if tol.TimeWindowSeconds > 0 && timeDiffSec > tol.TimeWindowSeconds {
				label := fmt.Sprintf("Settlement cutoff delay: %d seconds past window", timeDiffSec)
				brk := Break{
					ID:                uuid.New(),
					FileID:            fileID,
					SessionCode:       sessionCode,
					Channel:           channel,
					BreakType:         BreakTimingCutoff,
					BreakLabel:        label,
					Severity:          "MEDIUM",
					Status:            StatusUnmatched,
					InstructionRef:    sRow.InstructionRef,
					SessionID:         sRow.SessionID,
					LedgerAmountMinor: &t.AmountMinor,
					SwitchAmountMinor: &sRow.AmountMinor,
					AmountDiffMinor:   0,
					TimeDiffSeconds:   timeDiffSec,
					CreatedAt:         now,
					UpdatedAt:         now,
				}
				res.Breaks = append(res.Breaks, brk)
			} else {
				res.MatchedCount++
			}
		} else if absDiff <= tol.AmountToleranceMinor {
			// Within approved tolerance boundary
			res.ToleranceCount++
			st := StatusToleranceMatched
			if tol.AutoPairTolerance {
				st = StatusSettled
			}
			brk := Break{
				ID:                uuid.New(),
				FileID:            fileID,
				SessionCode:       sessionCode,
				Channel:           channel,
				BreakType:         BreakAmountMismatch,
				BreakLabel:        fmt.Sprintf("Tolerance paired: difference %s %d <= %d minor units", sRow.Currency, absDiff, tol.AmountToleranceMinor),
				Severity:          "LOW",
				Status:            st,
				InstructionRef:    sRow.InstructionRef,
				SessionID:         sRow.SessionID,
				LedgerAmountMinor: &t.AmountMinor,
				SwitchAmountMinor: &sRow.AmountMinor,
				AmountDiffMinor:   absDiff,
				TimeDiffSeconds:   int(math.Abs(sRow.SettledOn.Sub(t.LastEventAt).Seconds())),
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			res.Breaks = append(res.Breaks, brk)
		} else {
			// Beyond tolerance: AMOUNT_MISMATCH break
			sev := "HIGH"
			if absDiff >= 1_000_000 {
				sev = "CRITICAL"
			}
			label := fmt.Sprintf("Reconciliation amount discrepancy: ledger %d vs switch %d (delta %d)", t.AmountMinor, sRow.AmountMinor, absDiff)
			brk := Break{
				ID:                uuid.New(),
				FileID:            fileID,
				SessionCode:       sessionCode,
				Channel:           channel,
				BreakType:         BreakAmountMismatch,
				BreakLabel:        label,
				Severity:          sev,
				Status:            StatusUnmatched,
				InstructionRef:    sRow.InstructionRef,
				SessionID:         sRow.SessionID,
				LedgerAmountMinor: &t.AmountMinor,
				SwitchAmountMinor: &sRow.AmountMinor,
				AmountDiffMinor:   absDiff,
				TimeDiffSeconds:   int(math.Abs(sRow.SettledOn.Sub(t.LastEventAt).Seconds())),
				CreatedAt:         now,
				UpdatedAt:         now,
			}

			if m.Cases != nil {
				cID, cRef, err := m.openCase(ctx, tx, tenant, &t.ID, absDiff, sRow.Currency, channel, "settlement_amount_mismatch", sev, label, brk)
				if err == nil {
					brk.CaseID = cID
					brk.CaseRef = cRef
				}
			}

			res.Breaks = append(res.Breaks, brk)
		}
	}

	return res, nil
}

func (m *Matcher) openCase(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, txnID *uuid.UUID, exposure int64, curr, rail, root, risk, why string, brk Break) (*uuid.UUID, string, error) {
	c, created, err := m.Cases.Open(ctx, tx, tenant, cases.OpenInput{
		Classification: cases.CodeSettlement,
		Source:         cases.SourceBot,
		RiskGrade:      risk,
		TxnID:          txnID,
		ExposureMinor:  &exposure,
		Currency:       curr,
		Rail:           rail,
		RootCause:      root,
		RuleID:         RuleSettlement,
		RuleVersion:    RuleVersion,
		Explanation:    why,
		Actor:          cases.System,
		At:             m.now(),
		Evidence: []cases.Evidence{
			{
				Kind:         "reconciliation_break",
				Ref:          brk.ID.String(),
				SourceSystem: "BATCH_RECON_ENGINE",
				FetchedAt:    m.now(),
				IsFact:       true,
				Detail: map[string]any{
					"break_type":        brk.BreakType,
					"instruction_ref":   brk.InstructionRef,
					"amount_diff_minor": brk.AmountDiffMinor,
				},
			},
		},
	})
	if err != nil {
		return nil, "", err
	}
	if created {
		_, _ = notify.Record(ctx, tx, tenant, c.ID, notify.OpsExceptionOpened(c.Ref, cases.CodeSettlement, brk.InstructionRef, exposure, curr), c.Ref+":ops:opened")
	}
	return &c.ID, c.Ref, nil
}
