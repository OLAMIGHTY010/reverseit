package reconciliation

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var (
	ErrEmptyFile       = errors.New("settlement file is empty")
	ErrInvalidHeader   = errors.New("settlement file missing required columns")
	ErrInvalidRecord   = errors.New("settlement record has invalid fields")
	ErrUnsupportedKind = errors.New("unsupported settlement file kind")
)

// ParseSettlementFile reads a settlement CSV and returns normalized SettlementRows and the SHA-256 hash.
func ParseSettlementFile(r io.Reader, provider string) ([]SettlementRow, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", fmt.Errorf("reading settlement file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, "", ErrEmptyFile
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // variable allowed, checked per column index

	records, err := reader.ReadAll()
	if err != nil {
		return nil, "", fmt.Errorf("parsing settlement csv: %w", err)
	}
	if len(records) < 2 {
		return nil, hash, ErrEmptyFile
	}

	header := records[0]
	indices := resolveIndices(header)
	if indices.ref == -1 || indices.amount == -1 {
		return nil, hash, fmt.Errorf("%w: required instruction_ref and amount_minor", ErrInvalidHeader)
	}

	var rows []SettlementRow
	for lineIdx, row := range records[1:] {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		ref := getCol(row, indices.ref)
		if ref == "" {
			return nil, hash, fmt.Errorf("%w at line %d: instruction_ref is empty", ErrInvalidRecord, lineIdx+2)
		}
		amtStr := getCol(row, indices.amount)
		amt, err := strconv.ParseInt(amtStr, 10, 64)
		if err != nil || amt <= 0 {
			return nil, hash, fmt.Errorf("%w at line %d: invalid amount_minor %q", ErrInvalidRecord, lineIdx+2, amtStr)
		}

		curr := strings.ToUpper(getCol(row, indices.curr))
		if curr == "" {
			curr = "NGN"
		}
		sess := getCol(row, indices.sess)
		status := strings.ToUpper(getCol(row, indices.status))
		if status == "" {
			status = "SETTLED"
		}

		settledOn := parseDate(getCol(row, indices.date))

		rows = append(rows, SettlementRow{
			Provider:       provider,
			InstructionRef: ref,
			SessionID:      sess,
			AmountMinor:    amt,
			Currency:       curr,
			SettledOn:      settledOn,
			Status:         status,
		})
	}

	return rows, hash, nil
}

type colIndices struct {
	ref    int
	sess   int
	amount int
	curr   int
	date   int
	status int
}

func resolveIndices(header []string) colIndices {
	ci := colIndices{ref: -1, sess: -1, amount: -1, curr: -1, date: -1, status: -1}
	for i, col := range header {
		norm := strings.ToLower(strings.TrimSpace(col))
		switch norm {
		case "instruction_ref", "instructionref", "reference", "ref", "rrn", "stan":
			if ci.ref == -1 {
				ci.ref = i
			}
		case "session_id", "sessionid", "session":
			ci.sess = i
		case "amount_minor", "amountminor", "amount":
			if ci.amount == -1 {
				ci.amount = i
			}
		case "currency", "curr":
			ci.curr = i
		case "settled_on", "settledon", "cleared_at", "date", "timestamp":
			ci.date = i
		case "status", "clearing_status", "clearingstatus":
			ci.status = i
		}
	}
	return ci
}

func getCol(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02/01/2006",
		"02-01-2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}
