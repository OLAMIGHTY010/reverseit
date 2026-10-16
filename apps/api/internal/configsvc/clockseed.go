package configsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SeedClock is a candidate regulatory clock taken from the v2 prompt pack,
// section 5.3. The pack says these come from press coverage of CBN circulars, not
// from the primary documents, and must be checked with Wema Compliance before they
// drive anything a customer or regulator sees. They are therefore only ever
// proposed, never approved by the seeding code, and carry the status the pack gave
// them.
type SeedClock struct {
	ID           string
	Clock        Clock
	VerifyStatus string
	Source       string
	Note         string
}

const seedSource = "v2 prompt pack section 5.3 (checked 14 Oct 2026 against press reports)"

// SeedClocks lists the clocks the v2 pack proposes. Durations and statuses are
// the pack's. Start events are not always given there; where the pack is silent
// the start event is "unspecified" and the note says so.
var SeedClocks = []SeedClock{
	{"FAILED_INSTANT_TRANSFER", Clock{"Failed instant (NIP) transfer not reversed", 24, "hours", "failure_detected", "WAT", "Compliance (to be assigned)"},
		"reported", seedSource + ": CBN circular on instant interbank transfers, reported effective 2 Oct 2018",
		"Press ties the 24 hours to a complaint. The pack recommends starting at failure time, which is more protective. Decision D17."},
	{"WRONG_ACCOUNT_REQUEST", Clock{"Wrong account, wrong amount or duplicate transfer: written reversal request", 14, "working_days", "unspecified", "WAT", "Compliance (to be assigned)"},
		"reported", seedSource + ": same circular",
		"The pack gives 14 working days and does not name the start event."},
	{"WRONG_ACCOUNT_RECEIVING_ENTITY", Clock{"Receiving entity to comply once a written request is received, if funds are available", 1, "working_days", "request_received", "WAT", "Compliance (to be assigned)"},
		"reported", seedSource + ": same circular",
		"The indemnity detail affects complaint wording."},
	{"ATM_ON_US_MANUAL_REFUND", Clock{"Failed ATM, on-us: manual refund if automatic reversal fails", 24, "hours", "failure_detected", "WAT", "Compliance (to be assigned)"},
		"draft", seedSource + ": reported CBN statement of 8 June 2020 and newer ATM guidelines",
		"Final status of the newer guidelines is unconfirmed: some outlets say draft, some say issued."},
	{"ATM_NOT_ON_US", Clock{"Failed ATM, not-on-us", 48, "hours", "failure_detected", "WAT", "Compliance (to be assigned)"},
		"reported", seedSource + ": same 2020 statement",
		"48 hours is consistent across sources. Newer coverage mentions a 15 minute figure where automation works; verify."},
	{"POS_WEB_DISPUTE", Clock{"Disputed or failed POS or web transaction", 72, "hours", "dispute_reported", "WAT", "Compliance (to be assigned)"},
		"conflicting", seedSource + ": scope document says 72 hours",
		"Another summary of a 31 May 2020 letter says 48 hours. Keep 72 as configured, flag the conflict, ask Wema which applies. Decision D18."},
	{"INWARD_NIP_CREDIT", Clock{"Inward NIP credit applied to the beneficiary", 4, "minutes", "credit_received", "WAT", "Compliance (to be assigned)"},
		"reported", seedSource + ": same circular, reportedly about four minutes",
		"Not in the scope document. Added by the pack as exception type EX-DLY."},
}

// ClockKey returns the configuration key for a clock id.
func ClockKey(id string) string { return "regulatory_clock." + id }

// SeedClockProposals proposes every candidate clock. Each stays pending until a
// person other than maker approves it, so a fresh institution has no regulatory
// clock in force until Compliance has looked at each one.
func (s *Service) SeedClockProposals(ctx context.Context, tx pgx.Tx, tenant, maker uuid.UUID, effectiveFrom time.Time) ([]Version, error) {
	var out []Version
	for _, c := range SeedClocks {
		raw, err := json.Marshal(c.Clock)
		if err != nil {
			return nil, err
		}
		v, err := s.Propose(ctx, tx, tenant, maker, ProposeInput{
			Key: ClockKey(c.ID), Value: raw, SourceReference: c.Source, EffectiveFrom: effectiveFrom,
			VerifyStatus: c.VerifyStatus, Reason: "Seeded for verification: " + c.Note,
		})
		if err != nil {
			return nil, fmt.Errorf("seed clock %s: %w", c.ID, err)
		}
		out = append(out, v)
	}
	return out, nil
}
