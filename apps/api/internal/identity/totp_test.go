package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

// RFC 6238 appendix B: secret "12345678901234567890", SHA-1. The published
// 8-digit codes end in the 6-digit codes below.
func TestTOTPMatchesTheRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got := TOTPCode(secret, time.Unix(unix, 0)); got != want {
			t.Errorf("t=%d: got %s want %s", unix, got, want)
		}
	}
}

func TestMatchTOTPAllowsOneStepOfDriftOnly(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1111111109, 0)
	for _, off := range []int64{-30, 0, 30} {
		if _, ok := MatchTOTP(secret, TOTPCode(secret, now.Add(time.Duration(off)*time.Second)), now); !ok {
			t.Errorf("offset %d should be accepted", off)
		}
	}
	for _, off := range []int64{-90, 90} {
		if _, ok := MatchTOTP(secret, TOTPCode(secret, now.Add(time.Duration(off)*time.Second)), now); ok {
			t.Errorf("offset %d should be refused", off)
		}
	}
	if _, ok := MatchTOTP(secret, "12345", now); ok {
		t.Error("short code accepted")
	}
}

func (r *loginRig) enrol(t *testing.T, id interface{ String() string }, secret []byte) {
	t.Helper()
	if _, err := r.pool.Exec(context.Background(), `UPDATE users SET totp_secret = $2 WHERE id = $1::uuid`, id.String(), secret); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyAValidUnusedCodeYieldsAnMFAToken(t *testing.T) {
	r := newLoginRig(t)
	id := r.addUser(t, "approver@example.test", "SUPERVISOR")
	secret, _ := NewTOTPSecret()
	r.enrol(t, id, secret)
	ctx := context.Background()

	plain, err := r.auth.Login(ctx, r.code, "approver@example.test", goodPassword)
	if err != nil || plain.MFA {
		t.Fatalf("a login without a code must not carry MFA: %v %+v", err, plain)
	}
	if p, _ := r.verify.Verify(plain.Token, AudienceOps); p.MFA {
		t.Fatal("token claims MFA")
	}

	code := TOTPCode(secret, r.now)
	res, err := r.auth.LoginOTP(ctx, r.code, "approver@example.test", goodPassword, code)
	if err != nil || !res.MFA {
		t.Fatalf("valid code: %v %+v", err, res)
	}
	if p, _ := r.verify.Verify(res.Token, AudienceOps); !p.MFA {
		t.Fatal("token should carry MFA")
	}

	// The same code cannot be used again.
	if _, err := r.auth.LoginOTP(ctx, r.code, "approver@example.test", goodPassword, code); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("replay must fail, got %v", err)
	}
	// Wrong code, and a right code with the wrong password, both fail.
	if _, err := r.auth.LoginOTP(ctx, r.code, "approver@example.test", goodPassword, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong code: %v", err)
	}
	next := TOTPCode(secret, r.now.Add(30*time.Second))
	if _, err := r.auth.LoginOTP(ctx, r.code, "approver@example.test", "wrong password!!", next); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
}

func TestACodeForAUserWithoutAnAuthenticatorFailsInsteadOfDowngrading(t *testing.T) {
	r := newLoginRig(t)
	r.addUser(t, "nomfa@example.test", "SUPERVISOR")
	if _, err := r.auth.LoginOTP(context.Background(), r.code, "nomfa@example.test", goodPassword, "123456"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestWrongCodesCountTowardsLockout(t *testing.T) {
	r := newLoginRig(t)
	id := r.addUser(t, "lock@example.test", "SUPERVISOR")
	secret, _ := NewTOTPSecret()
	r.enrol(t, id, secret)
	for i := 0; i < 3; i++ {
		_, _ = r.auth.LoginOTP(context.Background(), r.code, "lock@example.test", goodPassword, "000000")
	}
	if _, locked := r.state(t, id); locked == nil {
		t.Fatal("three wrong codes should lock the account")
	}
}
