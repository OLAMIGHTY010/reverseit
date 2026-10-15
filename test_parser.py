from unittest.mock import call, patch

import parse
from regex_parser import parse_with_regex

# Define the sample alerts and expected results
samples = [
    {
        "alert": "Debit NGN200.00 from ExampleBank on 13/10/2026 at 14:05 via POS. Ref:12345",
        "expected": {
            "amount": 200.0,
            "date": "2026-10-13",
            "time": "14:05",
            "channel": "pos",
            "bank": "ExampleBank",
            "reference": "12345"
        }
    },
    {
        "alert": "Purchase of ₦500.50 made on 2026-10-12 09:30:00 through WEB at AnotherBank. Ref: ABC98765",
        "expected": {
            "amount": 500.5,
            "date": "2026-10-12",
            "time": "09:30",
            "channel": "web",
            "bank": "AnotherBank",
            "reference": "ABC98765"
        }
    },
    {
        "alert": "ATM withdrawal: NGN1,500.00 on 12-Oct-2026 15:30 from ThirdBank. Transaction ID: TX1234",
        "expected": {
            "amount": 1500.0,
            "date": "2026-10-12",
            "time": "15:30",
            "channel": "atm",
            "bank": "ThirdBank",
            "reference": "TX1234"
        }
    },
    {
        "alert": "Transfer sent: NGN3,000 on 13/10/2026 at 16:45 via Transfer from FourthBank to John Doe. Ref: TRF7890",
        "expected": {
            "amount": 3000.0,
            "date": "2026-10-13",
            "time": "16:45",
            "channel": "transfer",
            "bank": "FourthBank",
            "reference": "TRF7890"
        }
    },
    {
        "alert": "Web payment of ₦1234.56 successful on 2026-10-13 11:22:33 with FifthBank. ID: XYZ123",
        "expected": {
            "amount": 1234.56,
            "date": "2026-10-13",
            "time": "11:22",
            "channel": "web",
            "bank": "FifthBank",
            "reference": "XYZ123"
        }
    }
]

def test_parse_alert():
    expected_by_alert = {sample["alert"]: sample["expected"] for sample in samples}

    def fake_llm_fallback(alert):
        return expected_by_alert[alert]

    with patch("parse.parse_with_llm", side_effect=fake_llm_fallback) as llm_fallback:
        for sample in samples:
            alert = sample["alert"]
            expected = sample["expected"]

            regex_result = parse_with_regex(alert)
            assert regex_result is not None
            assert regex_result["bank"] == "Unknown"
            assert {
                key: value for key, value in regex_result.items() if key != "bank"
            } == {
                key: value for key, value in expected.items() if key != "bank"
            }

            assert parse.parse_alert(alert) == expected

    assert llm_fallback.call_args_list == [call(sample["alert"]) for sample in samples]


def test_llm_fallback_when_regex_fails():
    sample = samples[0]
    with (
        patch("parse.parse_with_regex", return_value=None),
        patch("parse.parse_with_llm", return_value=sample["expected"]) as llm_fallback,
    ):
        assert parse.parse_alert(sample["alert"]) == sample["expected"]
        llm_fallback.assert_called_once_with(sample["alert"])


if __name__ == "__main__":
    test_parse_alert()
    test_llm_fallback_when_regex_fails()
    print(f"Passed {len(samples)}/{len(samples)} regex + LLM fallback tests")