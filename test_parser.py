from regex_parser import parse_alert

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
    passed = 0
    total = len(samples)
    for i, sample in enumerate(samples, start=1):
        result = parse_alert(sample["alert"])
        print(f"Sample {i}:")
        print(f"  Alert: {sample['alert']}")
        print(f"  Result: {result}")
        if "error" in result:
            print(f"  Status: FAILED (error)")
            print(f"  Missing: {result.get('missing', [])}")
        else:
            # Compare each field
            mismatches = []
            for key, expected_val in sample["expected"].items():
                if key not in result:
                    mismatches.append(f"{key}: missing")
                else:
                    actual_val = result[key]
                    # For amount, we might have float vs int, but we expect float in expected
                    if isinstance(expected_val, float) and isinstance(actual_val, float):
                        if abs(expected_val - actual_val) > 0.001:
                            mismatches.append(f"{key}: expected {expected_val}, got {actual_val}")
                    else:
                        if expected_val != actual_val:
                            mismatches.append(f"{key}: expected {expected_val}, got {actual_val}")
            if mismatches:
                print(f"  Status: FAILED (mismatch)")
                for m in mismatches:
                    print(f"    - {m}")
            else:
                print(f"  Status: PASSED")
                passed += 1
        print()
    print(f"Passed {passed}/{total} tests")
    return passed == total

if __name__ == "__main__":
    test_parse_alert()