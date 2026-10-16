package datashield

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func mustKey(t *testing.T) string {
	t.Helper()
	k, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNormalizeAndMask(t *testing.T) {
	for in, want := range map[string]string{
		"0123456789":     "0123456789",
		" 0123-4567 89 ": "0123456789",
		"ab12cd34ef":     "AB12CD34EF",
	} {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "12345", "123456789012345678901", "0123 45678!", "ünïcode123"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) accepted", bad)
		}
	}
	if got := Mask("0123456789"); got != "****6789" {
		t.Errorf("Mask = %q", got)
	}
	// Invalid input is hidden entirely, never echoed.
	if got := Mask("not an account!"); got != "****" {
		t.Errorf("Mask of invalid = %q", got)
	}
}

func TestTokensAreStableKeyedAndTenantScoped(t *testing.T) {
	tk, err := NewTokenizer(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	a, b := uuid.New(), uuid.New()

	t1, _ := tk.Token(a, "0123456789")
	t2, _ := tk.Token(a, " 0123-456789 ")
	if t1 != t2 {
		t.Error("the same account in different spellings got different tokens")
	}
	if t3, _ := tk.Token(b, "0123456789"); t3 == t1 {
		t.Error("two tenants share a token for one account")
	}
	if t4, _ := tk.Token(a, "0123456788"); t4 == t1 {
		t.Error("two accounts share a token")
	}
	if !strings.HasPrefix(t1, "acct_") || len(t1) != len("acct_")+32 {
		t.Errorf("token format: %s", t1)
	}
	if strings.Contains(t1, "0123456789") {
		t.Error("token contains the account number")
	}

	other, _ := NewTokenizer(mustKey(t))
	if t5, _ := other.Token(a, "0123456789"); t5 == t1 {
		t.Error("two keys produce the same token")
	}
	if _, err := tk.Token(uuid.Nil, "0123456789"); err == nil {
		t.Error("nil tenant accepted")
	}
	if _, err := tk.Token(a, "bad"); err == nil {
		t.Error("bad account accepted")
	}
}

func TestSealAndOpen(t *testing.T) {
	s, err := NewSealer("k1", mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	sealed, err := s.Seal(tenant, "acct_x", "0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("0123456789")) {
		t.Fatal("ciphertext contains the plaintext")
	}
	got, err := s.Open(tenant, "acct_x", sealed)
	if err != nil || got != "0123456789" {
		t.Fatalf("open: %q %v", got, err)
	}

	again, _ := s.Seal(tenant, "acct_x", "0123456789")
	if bytes.Equal(sealed, again) {
		t.Error("two seals of one value are identical: the nonce is not random")
	}
}

func TestSealedValuesAreBoundToTenantAndToken(t *testing.T) {
	s, _ := NewSealer("k1", mustKey(t))
	tenant := uuid.New()
	sealed, _ := s.Seal(tenant, "acct_x", "0123456789")

	if _, err := s.Open(uuid.New(), "acct_x", sealed); !errors.Is(err, ErrCannotOpen) {
		t.Errorf("another tenant opened it: %v", err)
	}
	if _, err := s.Open(tenant, "acct_y", sealed); !errors.Is(err, ErrCannotOpen) {
		t.Errorf("another token opened it: %v", err)
	}
	flipped := append([]byte(nil), sealed...)
	flipped[len(flipped)-1] ^= 1
	if _, err := s.Open(tenant, "acct_x", flipped); !errors.Is(err, ErrCannotOpen) {
		t.Errorf("a modified value opened: %v", err)
	}
	for _, short := range [][]byte{nil, {1, 2, 3}, sealed[:10]} {
		if _, err := s.Open(tenant, "acct_x", short); !errors.Is(err, ErrCannotOpen) {
			t.Errorf("a truncated value opened: %v", err)
		}
	}
	other, _ := NewSealer("k1", mustKey(t))
	if _, err := other.Open(tenant, "acct_x", sealed); !errors.Is(err, ErrCannotOpen) {
		t.Errorf("another key opened it: %v", err)
	}
}

func TestKeyValidation(t *testing.T) {
	for name, k := range map[string]string{"empty": "", "not base64": "###", "short": "c2hvcnQ=", "long": strings.Repeat("QUFB", 20)} {
		if _, err := NewTokenizer(k); err == nil {
			t.Errorf("tokenizer accepted %s", name)
		}
		if _, err := NewSealer("k1", k); err == nil {
			t.Errorf("sealer accepted %s", name)
		}
	}
	if _, err := NewSealer("", mustKey(t)); err == nil {
		t.Error("empty key id accepted")
	}
	if _, err := NewSealer(strings.Repeat("k", 65), mustKey(t)); err == nil {
		t.Error("long key id accepted")
	}
	s, _ := NewSealer("2026-10", mustKey(t))
	if s.KeyID() != "2026-10" {
		t.Error("key id lost")
	}
}

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		"debit of 0123456789 failed":         "debit of ****6789 failed",
		"card 4111111111111111 declined":     "card ****1111 declined",
		"two: 0123456789 and 9988776655":     "two: ****6789 and ****6655",
		"11 digits are a phone: 08012345678": "11 digits are a phone: 08012345678",
		"short 123456789 stays":              "short 123456789 stays",
		"reference NIP202610001234":          "reference NIP202610001234",
		"amount 1850000 kobo":                "amount 1850000 kobo",
		"":                                   "",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
