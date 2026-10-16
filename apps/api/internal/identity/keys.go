// Package identity issues and verifies tokens and holds the authenticated
// principal. Tokens are Ed25519 (EdDSA) JWTs. Verifiers hold public keys only,
// so only the issuing process can create a token (ADR 0003, D5).
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Signer creates tokens. Only the issuing process holds one.
type Signer struct {
	kid string
	key ed25519.PrivateKey
	ttl time.Duration
}

// KeySet maps a key id to the public key that verifies tokens signed under it.
// Several keys can be present at once so a key can be rotated without
// invalidating live tokens.
type KeySet struct {
	keys map[string]ed25519.PublicKey
}

// GenerateKey returns a new key pair as base64 strings: the 32-byte private
// seed and the 32-byte public key.
func GenerateKey() (privateSeed, public string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub), nil
}

// NewSigner builds a Signer from a base64 private key. Both the 32-byte seed
// and the 64-byte expanded form are accepted.
func NewSigner(kid, privateB64 string, ttl time.Duration) (*Signer, error) {
	if !validKID(kid) {
		return nil, errors.New("identity: key id must be 1 to 64 characters from A-Z a-z 0-9 . _ -")
	}
	if ttl <= 0 || ttl > MaxTokenLifetime {
		return nil, fmt.Errorf("identity: token lifetime must be between 1s and %s", MaxTokenLifetime)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateB64))
	if err != nil {
		return nil, errors.New("identity: private key is not valid base64")
	}
	var key ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize:
		key = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize:
		key = ed25519.PrivateKey(raw)
	default:
		return nil, fmt.Errorf("identity: private key must be %d or %d bytes", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
	return &Signer{kid: kid, key: key, ttl: ttl}, nil
}

// KeyID returns the id placed in the header of every token this signer issues.
func (s *Signer) KeyID() string { return s.kid }

// PublicKey returns the verification key that matches this signer.
func (s *Signer) PublicKey() ed25519.PublicKey {
	return s.key.Public().(ed25519.PublicKey)
}

// ParseKeySet builds a KeySet from "kid:base64,kid:base64". It adds extra, which
// may be nil, such as the issuer's own public key.
func ParseKeySet(list string, extra map[string]ed25519.PublicKey) (*KeySet, error) {
	ks := &KeySet{keys: map[string]ed25519.PublicKey{}}
	for kid, k := range extra {
		ks.keys[kid] = k
	}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		kid, b64, ok := strings.Cut(item, ":")
		if !ok || !validKID(kid) {
			return nil, errors.New("identity: public keys must look like kid:base64")
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("identity: public key %q is not a base64 Ed25519 key", kid)
		}
		if existing, dup := ks.keys[kid]; dup && !existing.Equal(ed25519.PublicKey(raw)) {
			return nil, fmt.Errorf("identity: key id %q is given two different keys", kid)
		}
		ks.keys[kid] = ed25519.PublicKey(raw)
	}
	if len(ks.keys) == 0 {
		return nil, errors.New("identity: no verification keys configured")
	}
	return ks, nil
}

func (ks *KeySet) get(kid string) (ed25519.PublicKey, bool) {
	k, ok := ks.keys[kid]
	return k, ok
}

func validKID(kid string) bool {
	if kid == "" || len(kid) > 64 {
		return false
	}
	for _, r := range kid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
