// Command keygen prints new secrets for a deployment: the Ed25519 signing key
// for the token issuer, and the keys for account tokens and the sealed vault.
//
// The private value goes in JWT_PRIVATE_KEY of the API process only. The public
// value can be given to any process that must verify tokens, in JWT_PUBLIC_KEYS.
// Do not commit either to the repository.
package main

import (
	"fmt"
	"os"

	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
)

func main() {
	priv, pub, err := identity.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "keygen:", err)
		os.Exit(1)
	}
	tokenKey, err := datashield.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "keygen:", err)
		os.Exit(1)
	}
	encKey, err := datashield.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "keygen:", err)
		os.Exit(1)
	}
	fmt.Printf("JWT_PRIVATE_KEY=%s\n", priv)
	fmt.Printf("# public key, for JWT_PUBLIC_KEYS as kid:value\n# %s\n", pub)
	fmt.Printf("ACCOUNT_TOKEN_KEY=%s\n", tokenKey)
	fmt.Printf("ACCOUNT_ENC_KEY=%s\n", encKey)
}
