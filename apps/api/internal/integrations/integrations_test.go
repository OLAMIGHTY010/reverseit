package integrations

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerTripsAndRecovers(t *testing.T) {
	fakeNow := time.Now()
	cb := NewCircuitBreaker("test-switch", 3, 10*time.Second)
	cb.now = func() time.Time { return fakeNow }

	// First failure
	_ = cb.Execute(func() error { return errors.New("timeout") })
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", cb.State())
	}

	// Second failure
	_ = cb.Execute(func() error { return errors.New("timeout") })
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", cb.State())
	}

	// Third failure trips breaker to OPEN
	_ = cb.Execute(func() error { return errors.New("timeout") })
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after 3 failures, got %v", cb.State())
	}

	// Call while OPEN fails fast with ErrCircuitOpen without executing the func
	executed := false
	err := cb.Execute(func() error {
		executed = true
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if executed {
		t.Fatal("func should not have executed while circuit is open")
	}

	// Advance time past resetTimeout: breaker should transition to HalfOpen
	fakeNow = fakeNow.Add(11 * time.Second)
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen after timeout, got %v", cb.State())
	}

	// Successful call in HalfOpen closes the circuit
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after success in HalfOpen, got %v", cb.State())
	}
}

func TestNIPRailValidationAndResponseCodes(t *testing.T) {
	nip := &NIPRail{}
	if nip.Rail() != "NIP" {
		t.Errorf("expected NIP, got %s", nip.Rail())
	}

	// Session ID must be 30 numeric digits
	validSessionID := "999001261015090000000000123456"
	if err := nip.ValidateRef(validSessionID); err != nil {
		t.Errorf("expected valid session ID, got err: %v", err)
	}
	if err := nip.ValidateRef("12345"); err == nil {
		t.Error("expected error for short session ID")
	}
	if err := nip.ValidateRef("99900126101509000000000012345X"); err == nil {
		t.Error("expected error for non-numeric session ID")
	}

	// Code translations
	tests := []struct {
		code        string
		wantOutcome string
	}{
		{"00", OutcomeConfirmedSuccess},
		{"09", OutcomePending},
		{"91", OutcomeUnknown},
		{"96", OutcomeUnknown},
		{"51", OutcomeConfirmedFailure},
		{"58", OutcomeConfirmedFailure},
	}
	for _, tc := range tests {
		out := nip.TranslateResponse(tc.code)
		if out.Outcome != tc.wantOutcome {
			t.Errorf("code %s: want %s, got %s", tc.code, tc.wantOutcome, out.Outcome)
		}
	}
}

func TestCardSwitchRail(t *testing.T) {
	card := &CardSwitchRail{}
	if card.Rail() != "CARD" {
		t.Errorf("expected CARD, got %s", card.Rail())
	}

	if err := card.ValidateRef("123456789012"); err != nil {
		t.Errorf("valid RRN failed: %v", err)
	}
	if err := card.ValidateRef(""); err == nil {
		t.Error("empty RRN should fail")
	}

	out := card.TranslateResponse("00")
	if out.Outcome != OutcomeConfirmedSuccess || !out.IsTerminal {
		t.Errorf("00 expected terminal success, got %+v", out)
	}

	out68 := card.TranslateResponse("68")
	if out68.Outcome != OutcomeUnknown || out68.IsTerminal {
		t.Errorf("68 expected non-terminal unknown, got %+v", out68)
	}
}

func TestTelecomAdapter(t *testing.T) {
	ta := &TelecomAdapter{Provider: TelcoMTN}

	// Phone number validation
	if err := ta.ValidatePhone("08031234567"); err != nil {
		t.Errorf("valid phone failed: %v", err)
	}
	if err := ta.ValidatePhone("+2348021234567"); err != nil {
		t.Errorf("valid intl phone failed: %v", err)
	}
	if err := ta.ValidatePhone("123456"); err == nil {
		t.Error("invalid phone should fail")
	}

	// Carrier detection
	carriers := map[string]string{
		"08031234567": "MTN",
		"08021234567": "AIRTEL",
		"08051234567": "GLO",
		"08091234567": "9MOBILE",
	}
	for phone, want := range carriers {
		if got := ta.DetectCarrier(phone); got != want {
			t.Errorf("phone %s: want %s, got %s", phone, want, got)
		}
	}

	// Vendor drop translation
	drop := ta.TranslateOutcome("TIMEOUT")
	if !drop.IsVendorDrop || drop.Outcome != OutcomeUnknown {
		t.Errorf("TIMEOUT should be vendor drop unknown, got %+v", drop)
	}
	qualifies, reason := ta.TelecomReversalCandidate(drop)
	if !qualifies || reason == "" {
		t.Errorf("vendor drop should qualify for reversal, got %v (%s)", qualifies, reason)
	}
}
