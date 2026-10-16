package integrations

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var nigerianPhoneRegex = regexp.MustCompile(`^(?:\+234|234|0)(?:70|80|81|90|91)\d{8}$`)

// TelcoProvider identifies the supported mobile operators and aggregators.
type TelcoProvider string

const (
	TelcoMTN         TelcoProvider = "MTN"
	TelcoAirtel      TelcoProvider = "AIRTEL"
	TelcoGlo         TelcoProvider = "GLO"
	Telco9mobile     TelcoProvider = "9MOBILE"
	AggregatorCoral  TelcoProvider = "CORALPAY"
	AggregatorQuick  TelcoProvider = "QUICKTELLER"
	AggregatorUpper  TelcoProvider = "UPPERLINK"
)

// TelecomOutcome extends standard outcomes with telecom vendor specific attributes (Prompt B11).
type TelecomOutcome struct {
	StandardOutcome
	IsVendorDrop bool   `json:"is_vendor_drop"`
	Carrier      string `json:"carrier,omitempty"`
}

// TelecomAdapter handles VTU, data bundles, and VAS bill payment translations.
type TelecomAdapter struct {
	Provider TelcoProvider
}

// ValidatePhone validates that the recipient phone number adheres to the Nigerian telecom numbering plan.
func (ta *TelecomAdapter) ValidatePhone(phone string) error {
	phone = strings.TrimSpace(strings.ReplaceAll(phone, " ", ""))
	if !nigerianPhoneRegex.MatchString(phone) {
		return fmt.Errorf("telecom: invalid phone number %q; must be valid Nigerian mobile number", phone)
	}
	return nil
}

// DetectCarrier identifies the carrier from MSISDN prefix.
func (ta *TelecomAdapter) DetectCarrier(phone string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(phone, " ", ""))
	if strings.HasPrefix(cleaned, "+234") {
		cleaned = "0" + cleaned[4:]
	} else if strings.HasPrefix(cleaned, "234") {
		cleaned = "0" + cleaned[3:]
	}
	if len(cleaned) < 4 {
		return "UNKNOWN"
	}
	prefix := cleaned[:4]
	switch prefix {
	case "0803", "0806", "0703", "0706", "0813", "0816", "0810", "0814", "0903", "0906", "0913", "0916":
		return "MTN"
	case "0802", "0808", "0708", "0812", "0701", "0902", "0901", "0907", "0912":
		return "AIRTEL"
	case "0805", "0807", "0705", "0815", "0811", "0905", "0915":
		return "GLO"
	case "0809", "0817", "0818", "0909", "0908":
		return "9MOBILE"
	default:
		return "UNKNOWN"
	}
}

// TranslateOutcome maps telecom aggregator and telco responses into normalized platform outcomes.
func (ta *TelecomAdapter) TranslateOutcome(rawCode string) TelecomOutcome {
	upper := strings.ToUpper(strings.TrimSpace(rawCode))
	switch upper {
	case "00", "SUCCESS", "DELIVERED", "RECHARGED", "200":
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomeConfirmedSuccess,
				Description: "Airtime/Data successfully delivered to subscriber",
				RawCode:     rawCode,
				IsTerminal:  true,
			},
			IsVendorDrop: false,
		}

	case "PENDING", "PROCESSING", "IN_PROGRESS", "09":
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomePending,
				Description: "VAS request in-flight at telecom aggregator",
				RawCode:     rawCode,
				IsTerminal:  false,
			},
			IsVendorDrop: false,
		}

	case "TIMEOUT", "VENDOR_DROP", "GATEWAY_TIMEOUT", "NO_CARRIER_RESPONSE", "HTTP_504", "COMMUNICATION_ERROR", "91":
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomeUnknown,
				Description: "Telecom vendor dropped connection or timed out (VND001)",
				RawCode:     rawCode,
				IsTerminal:  false,
			},
			IsVendorDrop: true,
		}

	case "INVALID_NUMBER", "INVALID_MSISDN", "SUBSCRIBER_NOT_FOUND", "BARRED_SUBSCRIBER":
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomeConfirmedFailure,
				Description: "Invalid recipient phone number or subscriber line barred",
				RawCode:     rawCode,
				IsTerminal:  true,
			},
			IsVendorDrop: false,
		}

	case "INSUFFICIENT_VENDOR_BALANCE", "WALLET_EXHAUSTED":
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomeConfirmedFailure,
				Description: "Aggregator float balance exhausted",
				RawCode:     rawCode,
				IsTerminal:  true,
			},
			IsVendorDrop: true,
		}

	default:
		return TelecomOutcome{
			StandardOutcome: StandardOutcome{
				Outcome:     OutcomeUnknown,
				Description: "Unrecognized telecom status code: " + rawCode,
				RawCode:     rawCode,
				IsTerminal:  false,
			},
			IsVendorDrop: false,
		}
	}
}

// TelecomReversalCandidate assesses whether a failed telecom transaction qualifies for automated reversal.
func (ta *TelecomAdapter) TelecomReversalCandidate(outcome TelecomOutcome) (bool, string) {
	if outcome.IsVendorDrop {
		return true, "Vendor dropped request; debit confirmed on customer account without VAS delivery (VND001)"
	}
	if outcome.Outcome == OutcomeConfirmedFailure {
		return true, "Subscriber rejected by telco carrier; direct refund required"
	}
	return false, ""
}

// ValidateBillAmount ensures transaction amount is within regulatory and carrier bounds.
func (ta *TelecomAdapter) ValidateBillAmount(amountMinor int64) error {
	if amountMinor < 5000 { // Min 50 NGN
		return errors.New("telecom: minimum recharge amount is 50.00 NGN (5000 minor)")
	}
	if amountMinor > 5000000 { // Max 50,000 NGN per single transaction
		return errors.New("telecom: maximum single recharge exceeds regulatory limit of 50,000.00 NGN")
	}
	return nil
}
