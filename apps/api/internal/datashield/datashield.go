// Package datashield protects account numbers (ADR 0003, D10).
//
// Account numbers are stored and passed around as tokens. A token is a keyed
// HMAC of the number, so the same account always gets the same token inside a
// tenant (which is what correlation needs) and the number cannot be recovered
// from it. The original is kept sealed with AES-256-GCM in a vault and is
// revealed only through an audited, reason-gated call.
package datashield

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const keyLen = 32

// accountPattern accepts 6 to 20 letters and digits. Nigerian NUBAN account
// numbers are 10 digits; other rails use other shapes.
var accountPattern = regexp.MustCompile(`^[A-Za-z0-9]{6,20}$`)

// Normalize trims spaces and dashes and upper-cases an account number.
func Normalize(account string) (string, error) {
	n := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(account)))
	if !accountPattern.MatchString(n) {
		return "", errors.New("datashield: account number must be 6 to 20 letters or digits")
	}
	return n, nil
}

// Mask returns the number with all but the last four characters hidden. A
// number that is not valid is fully masked rather than echoed.
func Mask(account string) string {
	n, err := Normalize(account)
	if err != nil {
		return "****"
	}
	return "****" + n[len(n)-4:]
}

// Tokenizer turns account numbers into tokens.
type Tokenizer struct{ key []byte }

// NewTokenizer builds a Tokenizer from a base64 key of 32 bytes.
func NewTokenizer(keyB64 string) (*Tokenizer, error) {
	k, err := decodeKey(keyB64, "token key")
	if err != nil {
		return nil, err
	}
	return &Tokenizer{key: k}, nil
}

// Token returns the token for an account number inside a tenant. The tenant is
// part of the input, so the same number in two institutions gets different
// tokens.
func (t *Tokenizer) Token(tenant uuid.UUID, account string) (string, error) {
	n, err := Normalize(account)
	if err != nil {
		return "", err
	}
	if tenant == uuid.Nil {
		return "", errors.New("datashield: tenant id is required")
	}
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(tenant.String()))
	mac.Write([]byte{0})
	mac.Write([]byte(n))
	return "acct_" + hex.EncodeToString(mac.Sum(nil)[:16]), nil
}

// Sealer encrypts and decrypts account numbers.
type Sealer struct {
	keyID string
	aead  cipher.AEAD
}

// NewSealer builds a Sealer from a base64 AES-256 key. keyID is stored with
// each sealed value so keys can be rotated.
func NewSealer(keyID, keyB64 string) (*Sealer, error) {
	if keyID == "" || len(keyID) > 64 {
		return nil, errors.New("datashield: encryption key id must be 1 to 64 characters")
	}
	k, err := decodeKey(keyB64, "encryption key")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{keyID: keyID, aead: aead}, nil
}

// KeyID returns the id of the key this sealer uses.
func (s *Sealer) KeyID() string { return s.keyID }

// aad binds a ciphertext to its tenant and token, so a sealed value copied to
// another row or tenant fails to open.
func aad(tenant uuid.UUID, token string) []byte { return []byte(tenant.String() + "|" + token) }

// Seal encrypts account. The nonce is random and stored in front of the
// ciphertext.
func (s *Sealer) Seal(tenant uuid.UUID, token, account string) ([]byte, error) {
	n, err := Normalize(account)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("datashield: nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, []byte(n), aad(tenant, token)), nil
}

// Open decrypts a value made by Seal. Any failure, including a wrong tenant or
// token, returns the same error.
func (s *Sealer) Open(tenant uuid.UUID, token string, sealed []byte) (string, error) {
	ns := s.aead.NonceSize()
	if len(sealed) < ns+s.aead.Overhead() {
		return "", ErrCannotOpen
	}
	plain, err := s.aead.Open(nil, sealed[:ns], sealed[ns:], aad(tenant, token))
	if err != nil {
		return "", ErrCannotOpen
	}
	return string(plain), nil
}

// ErrCannotOpen is returned when a sealed value cannot be decrypted.
var ErrCannotOpen = errors.New("datashield: cannot open sealed value")

func decodeKey(b64, what string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("datashield: %s is not valid base64", what)
	}
	if len(k) != keyLen {
		return nil, fmt.Errorf("datashield: %s must be %d bytes", what, keyLen)
	}
	return k, nil
}

// GenerateKey returns a random 32-byte key as base64.
func GenerateKey() (string, error) {
	k := make([]byte, keyLen)
	if _, err := rand.Read(k); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k), nil
}

// Standalone runs of digits that look like an account number (ten digits, the
// NUBAN length) or a card number (13 to 19 digits). Digits inside a longer
// token, such as the reference NIP202610001234, are left alone so references
// stay useful in logs.
var digitRun = regexp.MustCompile(`\b[0-9]{10}\b|\b[0-9]{13,19}\b`)

// Redact masks anything in s that looks like an account or card number. It is a
// safety net for free text and log lines, where a number can turn up even though
// no code meant to put it there. It is not a substitute for tokenising.
func Redact(s string) string {
	return digitRun.ReplaceAllStringFunc(s, func(m string) string { return "****" + m[len(m)-4:] })
}
