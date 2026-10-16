package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
)

// IdempotencyHeader names the request header that carries the key.
const IdempotencyHeader = "Idempotency-Key"

var idempotencyKey = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

// ErrIdempotencyKeyRequired is returned when the header is absent.
var ErrIdempotencyKeyRequired = errors.New("Idempotency-Key header is required")

// IdempotencyKey returns the validated Idempotency-Key header. Endpoints that
// change financial state must call it and refuse requests without a key.
func IdempotencyKey(r *http.Request) (string, error) {
	k := r.Header.Get(IdempotencyHeader)
	if k == "" {
		return "", ErrIdempotencyKeyRequired
	}
	if !idempotencyKey.MatchString(k) {
		return "", errors.New("Idempotency-Key must be 1 to 200 characters from A-Z a-z 0-9 . _ : -")
	}
	return k, nil
}

// Fingerprint returns a stable hash of a request body. Store it with the key:
// the same key with the same fingerprint is a retry and returns the original
// result, the same key with a different fingerprint is a conflict (409).
func Fingerprint(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
