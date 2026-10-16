package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdempotencyKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := IdempotencyKey(r); err != ErrIdempotencyKeyRequired {
		t.Fatalf("missing key: %v", err)
	}

	for _, good := range []string{"a", "rev-INST-DEMO-01-NIP-DEMO-7781002-v1", "k:1.2_3", strings.Repeat("x", 200)} {
		r.Header.Set(IdempotencyHeader, good)
		if k, err := IdempotencyKey(r); err != nil || k != good {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
	for _, bad := range []string{"has space", "semi;colon", "new\nline", "ünï", strings.Repeat("x", 201)} {
		r.Header["Idempotency-Key"] = []string{bad}
		if _, err := IdempotencyKey(r); err == nil || err == ErrIdempotencyKeyRequired {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFingerprintIsStableAndSensitive(t *testing.T) {
	a := Fingerprint([]byte(`{"amount_minor":100}`))
	if a != Fingerprint([]byte(`{"amount_minor":100}`)) {
		t.Fatal("fingerprint not stable")
	}
	if a == Fingerprint([]byte(`{"amount_minor":101}`)) {
		t.Fatal("different bodies share a fingerprint")
	}
	if len(a) != 64 {
		t.Fatalf("length %d", len(a))
	}
}
