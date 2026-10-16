package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// MaxFileBytes bounds an uploaded file.
const MaxFileBytes = 32 << 20

// ErrUnreadable means the file as a whole cannot be read with the mapping, so none of
// it was imported.
var ErrUnreadable = errors.New("feed: the file cannot be read with this mapping")

// Result is what an import did.
type Result struct {
	FileID      uuid.UUID  `json:"file_id"`
	Mapping     string     `json:"mapping"`
	Version     int        `json:"mapping_version"`
	Status      string     `json:"status"`
	Total       int        `json:"rows_total"`
	Accepted    int        `json:"rows_accepted"`
	Duplicate   int        `json:"rows_duplicate"`
	Rejected    int        `json:"rows_rejected"`
	AlreadySeen bool       `json:"already_imported"`
	RowErrors   []RowError `json:"row_errors,omitempty"`
}

// Importer reads a file through an active mapping and ingests each row with the same
// service, and so the same rules, as an event delivered by API.
type Importer struct {
	Pool *db.Pool
	Txn  *txn.Service
}

// Import ingests a file. The same bytes are imported once: a second upload returns
// the first result. Each row is ingested in its own transaction, so one bad row never
// holds back the others, and a crash part way is safe to repeat because every event
// is idempotent. Rows that are refused are quarantined with the file and line.
func (im *Importer) Import(ctx context.Context, tenant uuid.UUID, mappingName string, data []byte) (Result, error) {
	if len(data) > MaxFileBytes {
		return Result{}, fmt.Errorf("%w: it is larger than %d bytes", ErrUnreadable, MaxFileBytes)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	var m *Mapping
	var prior *Result
	if err := im.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		if m, err = Active(ctx, tx, tenant, mappingName); err != nil {
			return err
		}
		prior, err = seenFile(ctx, tx, tenant, mappingName, hash)
		return err
	}); err != nil {
		return Result{}, err
	}
	if prior != nil {
		prior.AlreadySeen = true
		return *prior, nil
	}

	recs, rowErrs, err := m.Parse(data)
	if errors.Is(err, ErrMapping) {
		return Result{}, err
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnreadable, err.Error())
	}
	res := Result{Mapping: m.Name, Version: m.Version, Total: len(recs) + len(rowErrs), RowErrors: rowErrs}
	res.Rejected = len(rowErrs)
	for _, re := range rowErrs {
		if err := im.quarantine(ctx, tenant, m, hash, re.Line, re.Reason); err != nil {
			return Result{}, err
		}
	}
	for _, rec := range recs {
		var out txn.Result
		err := im.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			var err error
			out, err = im.Txn.Ingest(ctx, tx, tenant, rec.Envelope)
			return err
		})
		switch {
		case err == nil && out.Outcome == txn.Duplicate:
			res.Duplicate++
		case err == nil:
			res.Accepted++
		case errors.Is(err, txn.ErrInvalid), errors.Is(err, txn.ErrCoreMismatch), errors.Is(err, txn.ErrConflict):
			res.Rejected++
			res.RowErrors = append(res.RowErrors, RowError{Line: rec.Line, Reason: err.Error()})
			if qerr := im.quarantineEnvelope(ctx, tenant, m, rec, err.Error()); qerr != nil {
				return Result{}, qerr
			}
		default:
			return Result{}, fmt.Errorf("feed: line %d: %w", rec.Line, err)
		}
	}
	res.Status = "imported"
	if res.Rejected > 0 {
		res.Status = "imported_with_errors"
	}
	err = im.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO feed_files (tenant_id, mapping_name, mapping_version, file_hash, size_bytes, rows_total, rows_accepted, rows_duplicate, rows_rejected, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (tenant_id, mapping_name, file_hash) DO NOTHING RETURNING id`,
			tenant, m.Name, m.Version, hash, len(data), res.Total, res.Accepted, res.Duplicate, res.Rejected, res.Status).Scan(&res.FileID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Another import of the same bytes finished first; report its record.
			return tx.QueryRow(ctx, `SELECT id FROM feed_files WHERE tenant_id = $1 AND mapping_name = $2 AND file_hash = $3`, tenant, m.Name, hash).Scan(&res.FileID)
		}
		return err
	})
	return res, err
}

func (im *Importer) quarantine(ctx context.Context, tenant uuid.UUID, m *Mapping, hash string, line int, reason string) error {
	return im.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return im.Txn.Quarantine(ctx, tx, tenant, m.SourceSystem, fmt.Sprintf("feed %s v%d line %d: %s", m.Name, m.Version, line, reason), hash)
	})
}

// quarantineEnvelope keeps a refused event whole, so it can be replayed once the
// cause, such as the source's clock, is fixed.
func (im *Importer) quarantineEnvelope(ctx context.Context, tenant uuid.UUID, m *Mapping, rec Record, reason string) error {
	return im.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return im.Txn.QuarantineEnvelope(ctx, tx, tenant, rec.Envelope, fmt.Sprintf("feed %s v%d line %d: %s", m.Name, m.Version, rec.Line, reason))
	})
}

func seenFile(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, name, hash string) (*Result, error) {
	var r Result
	err := tx.QueryRow(ctx, `SELECT id, mapping_name, mapping_version, status, rows_total, rows_accepted, rows_duplicate, rows_rejected
		FROM feed_files WHERE tenant_id = $1 AND mapping_name = $2 AND file_hash = $3`, tenant, name, hash).
		Scan(&r.FileID, &r.Mapping, &r.Version, &r.Status, &r.Total, &r.Accepted, &r.Duplicate, &r.Rejected)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
