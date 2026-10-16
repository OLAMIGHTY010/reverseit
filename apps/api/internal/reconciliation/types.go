package reconciliation

import (
	"time"

	"github.com/google/uuid"
)

// Break type categories.
const (
	BreakLedgerNoSwitch = "LEDGER_DEBIT_NO_SWITCH"
	BreakSwitchNoLedger = "SWITCH_RECORD_NO_LEDGER"
	BreakAmountMismatch = "AMOUNT_MISMATCH"
	BreakTimingCutoff   = "TIMING_CUTOFF_BREAK"
)

// Break resolution statuses.
const (
	StatusUnmatched        = "UNMATCHED"
	StatusToleranceMatched = "TOLERANCE_MATCHED"
	StatusManuallyPaired   = "MANUALLY_PAIRED"
	StatusEscalatedDispute = "ESCALATED_DISPUTE"
	StatusSettled          = "SETTLED"
)

// Rule ID for reconciliation break detection.
const (
	RuleSettlement = "REC-STL-1"
	RuleVersion    = "1.0"
)

// SettlementFile represents an uploaded settlement, statement, or switch response file.
type SettlementFile struct {
	ID             uuid.UUID `json:"id"`
	Provider       string    `json:"provider"`
	Kind           string    `json:"kind"`
	PeriodStart    string    `json:"period_start"`
	PeriodEnd      string    `json:"period_end"`
	MappingVersion string    `json:"mapping_version"`
	SHA256         string    `json:"sha256"`
	Filename       string    `json:"filename"`
	RecordCount    int       `json:"record_count"`
	MatchedCount   int       `json:"matched_count"`
	BreakCount     int       `json:"break_count"`
	UploadedAt     time.Time `json:"uploaded_at"`
}

// SettlementRow is a normalized record parsed from a settlement file.
type SettlementRow struct {
	Provider       string    `json:"provider"`
	InstructionRef string    `json:"instruction_ref"`
	SessionID      string    `json:"session_id,omitempty"`
	AmountMinor    int64     `json:"amount_minor"`
	Currency       string    `json:"currency"`
	SettledOn      time.Time `json:"settled_on"`
	Status         string    `json:"status"`
}

// Break is an identified reconciliation discrepancy between core ledger and switch clearing.
type Break struct {
	ID                uuid.UUID  `json:"id"`
	FileID            *uuid.UUID `json:"file_id,omitempty"`
	SessionCode       string     `json:"session_code"`
	Channel           string     `json:"channel"`
	BreakType         string     `json:"break_type"`
	BreakLabel        string     `json:"break_label"`
	Severity          string     `json:"severity"`
	Status            string     `json:"status"`
	InstructionRef    string     `json:"instruction_ref"`
	SessionID         string     `json:"session_id,omitempty"`
	LedgerAmountMinor *int64     `json:"ledger_amount_minor,omitempty"`
	SwitchAmountMinor *int64     `json:"switch_amount_minor,omitempty"`
	AmountDiffMinor   int64      `json:"amount_diff_minor"`
	TimeDiffSeconds   int        `json:"time_diff_seconds"`
	CaseID            *uuid.UUID `json:"case_id,omitempty"`
	CaseRef           string     `json:"case_ref,omitempty"`
	MakerNotes        string     `json:"maker_notes,omitempty"`
	CheckerSignoff    any        `json:"checker_signoff,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ToleranceConfig sets boundaries within which slight discrepancies are auto-paired.
type ToleranceConfig struct {
	AmountToleranceMinor int64 `json:"amount_tolerance_minor"` // e.g. 500 (₦5.00)
	TimeWindowSeconds    int   `json:"time_window_seconds"`    // e.g. 1800 (30 min)
	AutoPairTolerance    bool  `json:"auto_pair_tolerance"`
}

// DefaultTolerance is the baseline reconciliation tolerance.
var DefaultTolerance = ToleranceConfig{
	AmountToleranceMinor: 500,  // ₦5.00
	TimeWindowSeconds:    1800, // 30 minutes
	AutoPairTolerance:    false,
}

// ReconSummary is the overall result of running reconciliation on a settlement file.
type ReconSummary struct {
	FileID            uuid.UUID `json:"file_id"`
	SHA256            string    `json:"sha256"`
	TotalRecords      int       `json:"total_records"`
	MatchedCount      int       `json:"matched_count"`
	ToleranceCount    int       `json:"tolerance_count"`
	BreakCount        int       `json:"break_count"`
	CasesOpenedCount  int       `json:"cases_opened_count"`
	NetDiscrepancyAmt int64     `json:"net_discrepancy_minor"`
}
