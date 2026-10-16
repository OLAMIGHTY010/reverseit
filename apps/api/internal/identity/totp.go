package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 specifies HMAC-SHA1 for compatibility with authenticator apps.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Development-only TOTP (RFC 6238, ADR 0003 D20). Six digits, 30 second steps,
// one step of clock drift allowed either way. A code is accepted once.
const (
	totpPeriod = 30
	totpDigits = 6
	totpDrift  = 1
)

// ErrNoTOTP means the user has not enrolled an authenticator.
var ErrNoTOTP = errors.New("identity: no authenticator enrolled")

// NewTOTPSecret returns a fresh 20-byte secret.
func NewTOTPSecret() ([]byte, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// TOTPCode computes the code for the step containing t.
func TOTPCode(secret []byte, t time.Time) string { return totpAt(secret, t.Unix()/totpPeriod) }

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%0*d", totpDigits, n%1_000_000)
}

// MatchTOTP returns the step a code belongs to, within the drift window, and
// whether it matched. It does not know whether the step was already used.
func MatchTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	found, ok := int64(0), false
	for d := int64(-totpDrift); d <= totpDrift; d++ {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, cur+d)), []byte(code)) == 1 {
			found, ok = cur+d, true
		}
	}
	return found, ok
}

// OTPAuthURI is the enrolment URI for an authenticator app. The issuer says the
// secret is synthetic so nobody mistakes it for a bank credential.
func OTPAuthURI(institution, email string, secret []byte) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	label := url.PathEscape("SMARTREVERSAL DEV " + institution + ":" + email)
	return "otpauth://totp/" + label + "?secret=" + enc + "&issuer=" + url.QueryEscape("SMARTREVERSAL DEV (synthetic)") +
		"&digits=6&period=30"
}

// TOTPStore keeps authenticator secrets. The PostgreSQL store implements it.
type TOTPStore interface {
	TOTPSecret(ctx context.Context, tenant, user uuid.UUID) ([]byte, error)
	// ConsumeTOTPStep records that step was used. It returns false when that
	// step or a later one was already used, so a code cannot be replayed.
	ConsumeTOTPStep(ctx context.Context, tenant, user uuid.UUID, step int64) (bool, error)
}

var _ TOTPStore = (*PGUserStore)(nil)

// TOTPSecret returns the user's secret, or ErrNoTOTP.
func (s *PGUserStore) TOTPSecret(ctx context.Context, tenant, user uuid.UUID) ([]byte, error) {
	var secret []byte
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT totp_secret FROM users WHERE id = $1`, user).Scan(&secret)
	})
	if err != nil {
		return nil, err
	}
	if len(secret) == 0 {
		return nil, ErrNoTOTP
	}
	return secret, nil
}

// ConsumeTOTPStep marks step used if it is newer than the last one.
func (s *PGUserStore) ConsumeTOTPStep(ctx context.Context, tenant, user uuid.UUID, step int64) (bool, error) {
	var n int64
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2`, user, step)
		n = tag.RowsAffected()
		return err
	})
	return n == 1, err
}

// EnrollTOTP stores a new secret for the user, replacing any earlier one.
func (s *PGUserStore) EnrollTOTP(ctx context.Context, tenant, user uuid.UUID, secret []byte) error {
	return s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE users SET totp_secret = $2, totp_last_step = 0 WHERE id = $1`, user, secret)
		return err
	})
}
