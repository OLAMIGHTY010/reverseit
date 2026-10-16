package identity

import (
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected format: %s", h)
	}
	ok, rehash, err := VerifyPassword("correct horse battery", h)
	if err != nil || !ok || rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = VerifyPassword("correct horse batterz", h)
	if err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v", ok, err)
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same password here")
	b, _ := HashPassword("same password here")
	if a == b {
		t.Fatal("two hashes of one password are identical")
	}
}

func TestPasswordLengthPolicy(t *testing.T) {
	if _, err := HashPassword(strings.Repeat("a", MinPasswordLength-1)); err != ErrPasswordPolicy {
		t.Errorf("short password: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("a", MaxPasswordLength+1)); err != ErrPasswordPolicy {
		t.Errorf("long password: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Errorf("minimum length rejected: %v", err)
	}
	h, _ := HashPassword(strings.Repeat("a", MinPasswordLength))
	if ok, _, err := VerifyPassword(strings.Repeat("a", MaxPasswordLength+1), h); ok || err != nil {
		t.Errorf("over-long attempt: ok=%v err=%v", ok, err)
	}
}

func TestVerifyFlagsWeakerParametersForRehash(t *testing.T) {
	// A hash made with lower memory still verifies, and asks for a rehash.
	weak := encodeHash(8*1024, 1, 1, []byte("0123456789abcdef"), argon2Key("some password!", []byte("0123456789abcdef"), 1, 8*1024, 1))
	ok, rehash, err := VerifyPassword("some password!", weak)
	if err != nil || !ok || !rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	good, _ := HashPassword("a perfectly fine one")
	parts := strings.Split(good, "$")
	cases := map[string]string{
		"empty":         "",
		"not a hash":    "plaintext",
		"wrong scheme":  strings.Replace(good, "argon2id", "argon2i", 1),
		"bad version":   strings.Replace(good, "v=19", "v=16", 1),
		"bad params":    strings.Replace(good, "m=19456,t=2,p=1", "m=x", 1),
		"zero params":   strings.Replace(good, "m=19456,t=2,p=1", "m=0,t=0,p=0", 1),
		"huge memory":   strings.Replace(good, "m=19456", "m=4000000", 1),
		"huge time":     strings.Replace(good, "t=2", "t=99", 1),
		"short salt":    "$" + strings.Join([]string{parts[1], parts[2], parts[3], "YWJj", parts[5]}, "$"),
		"bad salt":      "$" + strings.Join([]string{parts[1], parts[2], parts[3], "!!!", parts[5]}, "$"),
		"short value":   "$" + strings.Join([]string{parts[1], parts[2], parts[3], parts[4], "YWJj"}, "$"),
		"extra section": good + "$x",
	}
	for name, h := range cases {
		ok, _, err := VerifyPassword("a perfectly fine one", h)
		if ok || err == nil {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
}

// argon2Key builds a key with chosen parameters, to make hashes the current
// code would not produce.
func argon2Key(password string, salt []byte, t, m uint32, p uint8) []byte {
	return argon2.IDKey([]byte(password), salt, t, m, p, argonKeyLen)
}
