package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type rig struct {
	signer   *Signer
	verifier *Verifier
	now      time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	priv, _, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSigner("k1", priv, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ks, err := ParseKeySet("", map[string]ed25519.PublicKey{"k1": s.PublicKey()})
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{signer: s, now: t0}
	r.verifier = NewVerifier(ks, func() time.Time { return r.now })
	return r
}

func principal() Principal {
	return Principal{UserID: uuid.New(), TenantID: uuid.New(), Roles: []string{"REVERSAL_OFFICER"}, MFA: true}
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	r := newRig(t)
	p := principal()
	tok, exp, err := r.signer.Issue(p, AudienceOps, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(t0.Add(15 * time.Minute)) {
		t.Fatalf("expiry %s", exp)
	}
	got, err := r.verifier.Verify(tok, AudienceOps)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != p.UserID || got.TenantID != p.TenantID || !got.HasRole("REVERSAL_OFFICER") || !got.MFA ||
		got.Audience != AudienceOps || got.TokenID == "" {
		t.Fatalf("unexpected principal: %+v", got)
	}
	if got.HasRole("ADMIN") {
		t.Fatal("HasRole reports a role the token does not carry")
	}
}

func TestTokensCarryDistinctIDs(t *testing.T) {
	r := newRig(t)
	p := principal()
	a, _, _ := r.signer.Issue(p, AudienceOps, t0)
	b, _, _ := r.signer.Issue(p, AudienceOps, t0)
	pa, _ := r.verifier.Verify(a, AudienceOps)
	pb, _ := r.verifier.Verify(b, AudienceOps)
	if pa.TokenID == pb.TokenID {
		t.Fatal("two tokens share an id")
	}
}

func TestAudiencesAreNotInterchangeable(t *testing.T) {
	r := newRig(t)
	ops, _, _ := r.signer.Issue(principal(), AudienceOps, t0)
	cust, _, _ := r.signer.Issue(principal(), AudienceCustomer, t0)

	if _, err := r.verifier.Verify(ops, AudienceCustomer); err == nil {
		t.Error("operator token accepted on a customer route")
	}
	if _, err := r.verifier.Verify(cust, AudienceOps); err == nil {
		t.Error("customer token accepted on an operator route")
	}
	if _, err := r.verifier.Verify(cust, AudienceCustomer); err != nil {
		t.Errorf("customer token rejected on a customer route: %v", err)
	}
}

func TestExpiryAndNotBefore(t *testing.T) {
	r := newRig(t)
	tok, _, _ := r.signer.Issue(principal(), AudienceOps, t0)

	r.now = t0.Add(14 * time.Minute)
	if _, err := r.verifier.Verify(tok, AudienceOps); err != nil {
		t.Errorf("token rejected before expiry: %v", err)
	}
	r.now = t0.Add(15*time.Minute + leeway + time.Second)
	if _, err := r.verifier.Verify(tok, AudienceOps); err == nil {
		t.Error("expired token accepted")
	}
	r.now = t0.Add(-leeway - time.Minute)
	if _, err := r.verifier.Verify(tok, AudienceOps); err == nil {
		t.Error("token accepted before its not-before time")
	}
}

func TestWrongKeyAndTampering(t *testing.T) {
	r := newRig(t)
	tok, _, _ := r.signer.Issue(principal(), AudienceOps, t0)

	// Flip one character of the signature.
	parts := strings.Split(tok, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	if _, err := r.verifier.Verify(parts[0]+"."+parts[1]+"."+string(sig), AudienceOps); err == nil {
		t.Error("token with a damaged signature accepted")
	}

	// Swap the payload for another user's, keeping the old signature.
	other, _, _ := r.signer.Issue(principal(), AudienceOps, t0)
	op := strings.Split(other, ".")
	if _, err := r.verifier.Verify(parts[0]+"."+op[1]+"."+parts[2], AudienceOps); err == nil {
		t.Error("token with a swapped payload accepted")
	}

	// A different issuer's key under the same kid.
	priv2, _, _ := GenerateKey()
	s2, _ := NewSigner("k1", priv2, time.Minute)
	forged, _, _ := s2.Issue(principal(), AudienceOps, t0)
	if _, err := r.verifier.Verify(forged, AudienceOps); err == nil {
		t.Error("token signed by an unknown key accepted")
	}
}

func raw(t *testing.T, method jwt.SigningMethod, key any, header map[string]any, c claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, c)
	for k, v := range header {
		tok.Header[k] = v
	}
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func goodClaims() claims {
	return claims{
		TenantID: uuid.NewString(),
		Roles:    []string{"AUDITOR"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: Issuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{AudienceOps},
			IssuedAt: jwt.NewNumericDate(t0), NotBefore: jwt.NewNumericDate(t0),
			ExpiresAt: jwt.NewNumericDate(t0.Add(time.Minute)), ID: uuid.NewString(),
		},
	}
}

func TestAlgorithmConfusionAndWeakAlgorithmsAreRejected(t *testing.T) {
	r := newRig(t)
	pub := r.signer.PublicKey()

	// Classic attack: sign with HMAC using the public key as the secret.
	hs := raw(t, jwt.SigningMethodHS256, []byte(pub), map[string]any{"kid": "k1"}, goodClaims())
	if _, err := r.verifier.Verify(hs, AudienceOps); err == nil {
		t.Error("HS256 token keyed with the public key accepted")
	}
	// alg none.
	none := raw(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, map[string]any{"kid": "k1"}, goodClaims())
	if _, err := r.verifier.Verify(none, AudienceOps); err == nil {
		t.Error("unsigned token accepted")
	}
}

func TestKeyIDHandling(t *testing.T) {
	r := newRig(t)
	priv, _, _ := GenerateKey()
	seed, _ := base64.StdEncoding.DecodeString(priv)
	other := ed25519.NewKeyFromSeed(seed)

	for name, hdr := range map[string]map[string]any{
		"missing kid": {},
		"unknown kid": {"kid": "nope"},
		"empty kid":   {"kid": ""},
	} {
		tok := raw(t, jwt.SigningMethodEdDSA, other, hdr, goodClaims())
		if _, err := r.verifier.Verify(tok, AudienceOps); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestClaimValidation(t *testing.T) {
	r := newRig(t)
	// A dedicated signer, so only the claims differ between cases.
	priv, _, _ := GenerateKey()
	s, _ := NewSigner("k1", priv, time.Minute)
	r.verifier = NewVerifier(&KeySet{keys: map[string]ed25519.PublicKey{"k1": s.PublicKey()}}, func() time.Time { return t0 })
	sign := func(c claims) string { return raw(t, jwt.SigningMethodEdDSA, s.key, map[string]any{"kid": "k1"}, c) }

	mutations := map[string]func(*claims){
		"wrong issuer":      func(c *claims) { c.Issuer = "other" },
		"no issuer":         func(c *claims) { c.Issuer = "" },
		"no expiry":         func(c *claims) { c.ExpiresAt = nil },
		"no issued-at":      func(c *claims) { c.IssuedAt = nil },
		"no not-before":     func(c *claims) { c.NotBefore = nil },
		"no token id":       func(c *claims) { c.ID = "" },
		"bad subject":       func(c *claims) { c.Subject = "not-a-uuid" },
		"nil subject":       func(c *claims) { c.Subject = uuid.Nil.String() },
		"bad tenant":        func(c *claims) { c.TenantID = "x" },
		"nil tenant":        func(c *claims) { c.TenantID = uuid.Nil.String() },
		"no tenant":         func(c *claims) { c.TenantID = "" },
		"lowercase role":    func(c *claims) { c.Roles = []string{"admin"} },
		"role with space":   func(c *claims) { c.Roles = []string{"A B"} },
		"too many roles":    func(c *claims) { c.Roles = make([]string, maxRoles+1) },
		"lifetime too long": func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(t0.Add(MaxTokenLifetime + time.Hour)) },
	}
	if _, err := r.verifier.Verify(sign(goodClaims()), AudienceOps); err != nil {
		t.Fatalf("baseline claims rejected: %v", err)
	}
	for name, mutate := range mutations {
		c := goodClaims()
		mutate(&c)
		if _, err := r.verifier.Verify(sign(c), AudienceOps); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVerifyErrorDoesNotRevealWhy(t *testing.T) {
	r := newRig(t)
	for _, tok := range []string{"", "x", "a.b.c", strings.Repeat("a", 5000)} {
		_, err := r.verifier.Verify(tok, AudienceOps)
		if err != ErrInvalidToken {
			t.Errorf("error for %.10q is %v, want the generic error", tok, err)
		}
	}
}

func TestKeyRotationAcceptsBothKeysUntilOneIsRemoved(t *testing.T) {
	privOld, pubOld, _ := GenerateKey()
	privNew, _, _ := GenerateKey()
	oldS, _ := NewSigner("2026-09", privOld, time.Minute)
	newS, _ := NewSigner("2026-10", privNew, time.Minute)

	both, err := ParseKeySet("2026-09:"+pubOld, map[string]ed25519.PublicKey{"2026-10": newS.PublicKey()})
	if err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(both, func() time.Time { return t0 })
	a, _, _ := oldS.Issue(principal(), AudienceOps, t0)
	b, _, _ := newS.Issue(principal(), AudienceOps, t0)
	if _, err := v.Verify(a, AudienceOps); err != nil {
		t.Errorf("old key rejected during rotation: %v", err)
	}
	if _, err := v.Verify(b, AudienceOps); err != nil {
		t.Errorf("new key rejected: %v", err)
	}

	onlyNew, _ := ParseKeySet("", map[string]ed25519.PublicKey{"2026-10": newS.PublicKey()})
	v = NewVerifier(onlyNew, func() time.Time { return t0 })
	if _, err := v.Verify(a, AudienceOps); err == nil {
		t.Error("token from a removed key still accepted")
	}
}

func TestIssueRejectsBadInput(t *testing.T) {
	r := newRig(t)
	good := principal()
	bad := map[string]func(*Principal){
		"no user":   func(p *Principal) { p.UserID = uuid.Nil },
		"no tenant": func(p *Principal) { p.TenantID = uuid.Nil },
		"bad role":  func(p *Principal) { p.Roles = []string{"x"} },
		"too many":  func(p *Principal) { p.Roles = make([]string, maxRoles+1) },
	}
	for name, mutate := range bad {
		p := good
		mutate(&p)
		if _, _, err := r.signer.Issue(p, AudienceOps, t0); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, _, err := r.signer.Issue(good, "other", t0); err == nil {
		t.Error("unknown audience accepted")
	}
}

func TestKeyParsing(t *testing.T) {
	priv, pub, _ := GenerateKey()

	if _, err := NewSigner("bad kid!", priv, time.Minute); err == nil {
		t.Error("bad kid accepted")
	}
	if _, err := NewSigner("k", "###", time.Minute); err == nil {
		t.Error("non-base64 key accepted")
	}
	if _, err := NewSigner("k", base64.StdEncoding.EncodeToString([]byte("short")), time.Minute); err == nil {
		t.Error("short key accepted")
	}
	if _, err := NewSigner("k", priv, 0); err == nil {
		t.Error("zero lifetime accepted")
	}
	if _, err := NewSigner("k", priv, MaxTokenLifetime+time.Second); err == nil {
		t.Error("over-long lifetime accepted")
	}

	// Both the seed and the expanded private key are accepted and agree.
	seed, _ := base64.StdEncoding.DecodeString(priv)
	full := base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(seed))
	a, err1 := NewSigner("k", priv, time.Minute)
	b, err2 := NewSigner("k", full, time.Minute)
	if err1 != nil || err2 != nil || !a.PublicKey().Equal(b.PublicKey()) {
		t.Errorf("seed and expanded key disagree: %v %v", err1, err2)
	}
	if got := base64.StdEncoding.EncodeToString(a.PublicKey()); got != pub {
		t.Errorf("public key %s, generated %s", got, pub)
	}

	for name, list := range map[string]string{
		"no colon":     "k1",
		"bad kid":      "bad kid:" + pub,
		"bad base64":   "k1:###",
		"wrong length": "k1:" + base64.StdEncoding.EncodeToString([]byte("short")),
		"conflict":     "k1:" + pub + ",k1:" + mustPub(t),
	} {
		if _, err := ParseKeySet(list, nil); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := ParseKeySet("", nil); err == nil {
		t.Error("empty key set accepted")
	}
	if _, err := ParseKeySet(" k1:"+pub+" , ,", nil); err != nil {
		t.Errorf("whitespace and empty items should be tolerated: %v", err)
	}
}

func mustPub(t *testing.T) string {
	t.Helper()
	_, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestLoad(t *testing.T) {
	priv, _, _ := GenerateKey()
	if _, _, err := Load("k1", "", "", time.Minute); err == nil || !strings.Contains(err.Error(), "keygen") {
		t.Errorf("missing key should say how to generate one, got %v", err)
	}
	s, v, err := Load("k1", priv, "", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, _ := s.Issue(principal(), AudienceOps, time.Now())
	if _, err := v.Verify(tok, AudienceOps); err != nil {
		t.Errorf("issuer's own token rejected by its own verifier: %v", err)
	}
	if _, _, err := Load("k1", priv, "k1:"+mustPub(t), time.Minute); err == nil {
		t.Error("a different key under the issuer's own kid was accepted")
	}
	if _, _, err := Load("k1", "###", "", time.Minute); err == nil {
		t.Error("bad private key accepted")
	}
}
