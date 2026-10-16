package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters follow the OWASP minimum recommendation (19 MiB, 2
// passes, 1 lane). Raise them when hardware allows. Hashes carry their own
// parameters, so old hashes keep verifying and are flagged for rehash.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16

	// MinPasswordLength and MaxPasswordLength bound what is accepted. The upper
	// bound stops a request from forcing a very long hash computation.
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// ErrPasswordPolicy is returned when a password is too short or too long.
var ErrPasswordPolicy = fmt.Errorf("password must be %d to %d characters", MinPasswordLength, MaxPasswordLength)

// HashPassword returns an argon2id hash in PHC string format.
func HashPassword(password string) (string, error) {
	if n := len(password); n < MinPasswordLength || n > MaxPasswordLength {
		return "", ErrPasswordPolicy
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("identity: salt: %w", err)
	}
	return encodeHash(argonMemoryKiB, argonTime, argonThreads, salt,
		argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)), nil
}

func encodeHash(m uint32, t uint32, p uint8, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, m, t, p,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// VerifyPassword checks password against an argon2id hash in constant time. It
// reports whether the hash was made with weaker parameters than the current
// ones, so the caller can store a fresh hash after a successful login. A hash
// it cannot parse is an error, never a match.
func VerifyPassword(password, encoded string) (match, needsRehash bool, err error) {
	if len(password) > MaxPasswordLength {
		return false, false, nil
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, false, errors.New("identity: unsupported password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false, errors.New("identity: unsupported password hash version")
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false, false, errors.New("identity: bad password hash parameters")
	}
	if m > 1<<20 || t > 10 || p > 16 {
		return false, false, errors.New("identity: password hash parameters out of range")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false, false, errors.New("identity: bad password hash salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 {
		return false, false, errors.New("identity: bad password hash value")
	}

	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	match = subtle.ConstantTimeCompare(got, want) == 1
	needsRehash = m < argonMemoryKiB || t < argonTime || p < argonThreads
	return match, needsRehash, nil
}
