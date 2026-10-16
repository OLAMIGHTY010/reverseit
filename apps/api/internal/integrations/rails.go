package integrations

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Standard transaction outcomes across all rails.
const (
	OutcomeConfirmedSuccess = "CONFIRMED_SUCCESS"
	OutcomeConfirmedFailure = "CONFIRMED_FAILURE"
	OutcomePending          = "PENDING"
	OutcomeUnknown          = "UNKNOWN"
	OutcomeConflicting      = "CONFLICTING"
)

// StandardOutcome is the normalized result of a rail or switch response code.
type StandardOutcome struct {
	Outcome     string `json:"outcome"`
	Description string `json:"description"`
	RawCode     string `json:"raw_code"`
	IsTerminal  bool   `json:"is_terminal"`
}

// PaymentRail represents an adapter for a specific payment clearing switch or rail.
type PaymentRail interface {
	Rail() string
	TranslateResponse(rawCode string) StandardOutcome
	ValidateRef(ref string) error
}

// ---- NIBSS Instant Payments (NIP) Rail ----------------------------------------

type NIPRail struct{}

var nipSessionIDRegex = regexp.MustCompile(`^\d{30}$`)

func (n *NIPRail) Rail() string { return "NIP" }

// ValidateRef validates that a NIP session ID adheres to the 30-digit NIBSS specification.
func (n *NIPRail) ValidateRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if !nipSessionIDRegex.MatchString(ref) {
		return fmt.Errorf("nip: invalid session ID %q; must be exactly 30 numeric digits", ref)
	}
	return nil
}

// TranslateResponse maps NIBSS response codes to standard outcomes.
func (n *NIPRail) TranslateResponse(rawCode string) StandardOutcome {
	rawCode = strings.TrimSpace(rawCode)
	switch rawCode {
	case "00":
		return StandardOutcome{Outcome: OutcomeConfirmedSuccess, Description: "Approved or completed successfully", RawCode: rawCode, IsTerminal: true}
	case "09":
		return StandardOutcome{Outcome: OutcomePending, Description: "Request in progress; pending credit confirmation", RawCode: rawCode, IsTerminal: false}
	case "91":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Routing error or switch inoperative; destination indeterminate", RawCode: rawCode, IsTerminal: false}
	case "96":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "System malfunction; status indeterminate", RawCode: rawCode, IsTerminal: false}
	case "01":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Status unknown / unconfirmed at destination", RawCode: rawCode, IsTerminal: true}
	case "03":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Invalid sender bank or sender institution", RawCode: rawCode, IsTerminal: true}
	case "05":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Do not honor", RawCode: rawCode, IsTerminal: true}
	case "06":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Dormant account", RawCode: rawCode, IsTerminal: true}
	case "07":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Transfer limit exceeded", RawCode: rawCode, IsTerminal: true}
	case "08":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Account inactive", RawCode: rawCode, IsTerminal: true}
	case "12":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Invalid transaction reference", RawCode: rawCode, IsTerminal: true}
	case "25":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Unable to locate record on file", RawCode: rawCode, IsTerminal: true}
	case "51":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Insufficient funds", RawCode: rawCode, IsTerminal: true}
	case "58":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Transaction not permitted to terminal or account", RawCode: rawCode, IsTerminal: true}
	default:
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Unrecognized response code: " + rawCode, RawCode: rawCode, IsTerminal: false}
	}
}

// ---- Card Switch (Interswitch / Mastercard / Visa / Verve) Rail ----------------

type CardSwitchRail struct{}

func (c *CardSwitchRail) Rail() string { return "CARD" }

func (c *CardSwitchRail) ValidateRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if len(ref) < 6 || len(ref) > 36 {
		return errors.New("card: retrieval reference number (RRN) or transaction ID must be 6-36 chars")
	}
	return nil
}

// TranslateResponse maps ISO 8583 response codes to standard outcomes.
func (c *CardSwitchRail) TranslateResponse(rawCode string) StandardOutcome {
	rawCode = strings.TrimSpace(rawCode)
	switch rawCode {
	case "00":
		return StandardOutcome{Outcome: OutcomeConfirmedSuccess, Description: "Transaction approved", RawCode: rawCode, IsTerminal: true}
	case "68":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Response received too late; timeout at switch", RawCode: rawCode, IsTerminal: false}
	case "91":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Issuer or switch inoperative", RawCode: rawCode, IsTerminal: false}
	case "96":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "System malfunction; outcome uncertain", RawCode: rawCode, IsTerminal: false}
	case "05":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Do not honor", RawCode: rawCode, IsTerminal: true}
	case "14":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Invalid card number", RawCode: rawCode, IsTerminal: true}
	case "51":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Insufficient funds", RawCode: rawCode, IsTerminal: true}
	case "54":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Expired card", RawCode: rawCode, IsTerminal: true}
	case "55":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Incorrect PIN", RawCode: rawCode, IsTerminal: true}
	case "57":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Transaction not permitted for cardholder", RawCode: rawCode, IsTerminal: true}
	case "58":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Transaction not permitted to terminal", RawCode: rawCode, IsTerminal: true}
	case "61":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Exceeds withdrawal amount limit", RawCode: rawCode, IsTerminal: true}
	default:
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Unrecognized ISO 8583 response: " + rawCode, RawCode: rawCode, IsTerminal: false}
	}
}

// ---- Core Banking System (CBS: Finacle / T24 / Flexcube) Rail ------------------

type CoreBankingRail struct{}

func (cb *CoreBankingRail) Rail() string { return "CORE_BANKING" }

func (cb *CoreBankingRail) ValidateRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return errors.New("core_banking: transaction reference is required")
	}
	return nil
}

func (cb *CoreBankingRail) TranslateResponse(rawCode string) StandardOutcome {
	rawCode = strings.ToUpper(strings.TrimSpace(rawCode))
	switch rawCode {
	case "POSTED", "SUCCESS", "CONFIRMED":
		return StandardOutcome{Outcome: OutcomeConfirmedSuccess, Description: "Ledger posting verified", RawCode: rawCode, IsTerminal: true}
	case "PENDING_SETTLEMENT", "PENDING_CLEARING":
		return StandardOutcome{Outcome: OutcomePending, Description: "Pending core settlement processing", RawCode: rawCode, IsTerminal: false}
	case "INSUFFICIENT_FUNDS", "ACCOUNT_FROZEN", "ACCOUNT_CLOSED", "POSTING_RESTRICTED":
		return StandardOutcome{Outcome: OutcomeConfirmedFailure, Description: "Core banking rule blocked posting: " + rawCode, RawCode: rawCode, IsTerminal: true}
	case "TIMEOUT", "CONNECTION_REFUSED", "NODE_UNAVAILABLE":
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Core banking host connection timeout", RawCode: rawCode, IsTerminal: false}
	default:
		return StandardOutcome{Outcome: OutcomeUnknown, Description: "Unrecognized core banking response: " + rawCode, RawCode: rawCode, IsTerminal: false}
	}
}
