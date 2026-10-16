package identity

import (
	"crypto/ed25519"
	"errors"
	"time"
)

// Load builds the signer and verifier for a process that issues and checks
// tokens. The issuer's own public key is added to the verification set
// automatically. A missing private key is an error: the API cannot start
// without one, and the message says what to do.
func Load(kid, privateB64, publicList string, ttl time.Duration) (*Signer, *Verifier, error) {
	if privateB64 == "" {
		return nil, nil, errors.New("identity: JWT_PRIVATE_KEY is not set; generate one with: go run ./cmd/keygen")
	}
	signer, err := NewSigner(kid, privateB64, ttl)
	if err != nil {
		return nil, nil, err
	}
	keys, err := ParseKeySet(publicList, map[string]ed25519.PublicKey{signer.KeyID(): signer.PublicKey()})
	if err != nil {
		return nil, nil, err
	}
	return signer, NewVerifier(keys, nil), nil
}
