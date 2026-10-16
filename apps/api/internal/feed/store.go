package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoMapping means no active mapping of that name exists.
var ErrNoMapping = errors.New("feed: no active mapping with that name")

// ErrMappingExists means that version of the mapping is already stored. A stored
// version is never changed: a new layout is a new version.
var ErrMappingExists = errors.New("feed: that version of the mapping already exists")

// Save stores a new draft version of a mapping after validating it.
func Save(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, m *Mapping, by string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	def, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO feed_mappings (tenant_id, name, version, definition, created_by) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT DO NOTHING`, tenant, m.Name, m.Version, def, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMappingExists
	}
	return nil
}

// Activate makes one version the one in use and retires the previous one, in the
// caller's transaction.
func Activate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, name string, version int) error {
	if _, err := tx.Exec(ctx, `UPDATE feed_mappings SET status = 'retired' WHERE tenant_id = $1 AND name = $2 AND status = 'active' AND version <> $3`,
		tenant, name, version); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE feed_mappings SET status = 'active' WHERE tenant_id = $1 AND name = $2 AND version = $3 AND status <> 'retired'`,
		tenant, name, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Either it does not exist, or it is the version already active.
		var st string
		if err := tx.QueryRow(ctx, `SELECT status FROM feed_mappings WHERE tenant_id = $1 AND name = $2 AND version = $3`, tenant, name, version).Scan(&st); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoMapping
			}
			return err
		}
		if st == "retired" {
			return fmt.Errorf("feed: version %d was retired; save a new version instead", version)
		}
	}
	return nil
}

// Active loads the mapping in use.
func Active(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, name string) (*Mapping, error) {
	var def []byte
	err := tx.QueryRow(ctx, `SELECT definition FROM feed_mappings WHERE tenant_id = $1 AND name = $2 AND status = 'active'`, tenant, name).Scan(&def)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMapping
	}
	if err != nil {
		return nil, err
	}
	var m Mapping
	if err := json.Unmarshal(def, &m); err != nil {
		return nil, fmt.Errorf("feed: stored mapping is unreadable: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
