# Test table - letter generation

Manual test table for `generate_letter(kind, dispute)` and `generate_letters(dispute)`.
Every row below was also automated in [`test_letters.py`](test_letters.py) and
[`test_main.py`](test_main.py); the "Automated" column gives the test name.

Run it all with:

```bash
python -m pytest -q     # 30 passed
python demo.py          # prints both letters and checks for placeholders
```

Result: **30 passed, 0 failed** (Python 3.14, pytest 8, `ANTHROPIC_API_KEY` unset,
so the deterministic template path is what is exercised).

## 1. Output shape and the no-placeholder rule

| # | Input | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 1 | `generate_letter("bank", dispute)` | non-empty `str` | `str`, starts `"2026-10-13\n\nThe Complaints Desk\nExampleBank..."` | PASS | `test_generate_letter_returns_text[bank]` |
| 2 | `generate_letter("cbn", dispute)` | non-empty `str` | `str`, starts `"2026-10-13\n\nThe Director, Consumer Protection Department..."` | PASS | `test_generate_letter_returns_text[cbn]` |
| 3 | bank letter text | no `{placeholder}` left | `find_unfilled_placeholders() == []`; no `{word}` matches | PASS | `test_no_unfilled_placeholders[bank]` |
| 4 | CBN letter text | no `{placeholder}` left | `find_unfilled_placeholders() == []`; no `{word}` matches | PASS | `test_no_unfilled_placeholders[cbn]` |
| 5 | both letters | every dispute fact present | `NGN 20,000`, `2026-10-13`, `2026-10-27`, `ExampleBank`, `12345`, `point-of-sale (POS) transaction` all found | PASS | `test_letter_contains_every_dispute_fact` |
| 6 | `generate_letters(dispute)` | both kinds, different text | keys `{"bank","cbn"}`, texts differ | PASS | `test_letters_are_different` |

## 2. Addressing

| # | Input | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 7 | bank letter | addressed to the bank | contains `The Complaints Desk` and `ExampleBank` | PASS | `test_bank_letter_is_addressed_to_the_bank` |
| 8 | CBN letter | addressed to the CBN | contains `Central Bank of Nigeria` and `Consumer Protection` | PASS | `test_cbn_letter_is_addressed_to_the_cbn` |
| 9 | `kind="BANK"` | case insensitive | identical to `kind="bank"` | PASS | `test_kind_is_case_insensitive` |
| 10 | `kind="police"` | rejected | raises `ValueError` | PASS | `test_unknown_kind_is_rejected` |

## 3. Bad dispute records must fail loudly, not print a broken letter

| # | Input (field removed / blanked) | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 11 | `amount` missing | `MissingDisputeFieldError`, `missing == ["amount"]` | raised, field named | PASS | `test_missing_field_is_rejected[amount]` |
| 12 | `date` missing | `MissingDisputeFieldError` | raised, field named | PASS | `test_missing_field_is_rejected[date]` |
| 13 | `channel` missing | `MissingDisputeFieldError` | raised, field named | PASS | `test_missing_field_is_rejected[channel]` |
| 14 | `bank` missing | `MissingDisputeFieldError` | raised, field named | PASS | `test_missing_field_is_rejected[bank]` |
| 15 | `reference` missing | `MissingDisputeFieldError` | raised, field named | PASS | `test_missing_field_is_rejected[reference]` |
| 16 | `deadline` missing | `MissingDisputeFieldError` | raised: `dispute record is missing required field(s): deadline` | PASS | `test_missing_field_is_rejected[deadline]` |
| 17 | `reference="   "` (whitespace) | treated as missing | `MissingDisputeFieldError` | PASS | `test_blank_field_is_rejected` |

## 4. Amount and channel formatting

| # | Input | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 18 | `amount=20000` | `NGN 20,000` | `NGN 20,000` | PASS | `test_amount_is_formatted` |
| 19 | `amount=20000.0` | `NGN 20,000` | `NGN 20,000` | PASS | `test_amount_is_formatted` |
| 20 | `amount=20000.5` | `NGN 20,000.50` | `NGN 20,000.50` | PASS | `test_amount_is_formatted` |
| 21 | `amount="NGN20,000.00"` | `NGN 20,000` | `NGN 20,000` | PASS | `test_amount_is_formatted` |
| 22 | `amount="₦20,000.00"` | `NGN 20,000` | `NGN 20,000` | PASS | `test_amount_is_formatted` |
| 23 | `channel` in `atm`/`pos`/`transfer`/`web` | readable prose | `an ATM withdrawal` / `a point-of-sale (POS) transaction` / `a bank transfer` / `an online (web) transaction` | PASS | `test_channels_render_as_prose` |
| 24 | `channel="ussd"` (unexpected) | falls back to the raw name | contains `ussd transaction` | PASS | `test_unknown_channel_falls_back_to_its_name` |

## 5. LLM polish must never invent facts

`_polish_with_llm` is monkeypatched here, so the guard is tested without a
network call. In production the polished text is only accepted if it keeps
every fact and adds no new figure, date or rule.

| # | Input (fake model response) | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 25 | polish returns the letter plus a newline | polished text is used | returned text ends with `"\n"`, no placeholders | PASS | `test_polished_letter_is_used_when_it_keeps_the_facts` |
| 26 | polish drops the amount and reference (returns a short paragraph) | rejected, template kept | `NGN 20,000` and `12345` still in output | PASS | `test_polish_that_drops_a_fact_is_discarded` |
| 27 | polish adds "Under section 12 of the Banks Act, you owe me NGN 50,000" | rejected, template kept | neither `NGN 50,000` nor `section 12` in output | PASS | `test_polish_that_invents_a_rule_is_discarded` |
| 28 | polish returns `"COMPLAINT ABOUT {amount} ON {date}"` | rejected, template kept | `find_unfilled_placeholders() == []` | PASS | `test_polish_with_placeholders_is_discarded` |

## 6. API skeleton (unchanged, still passing)

| # | Input | Expected result | Actual result | Pass/Fail | Automated |
| --- | --- | --- | --- | --- | --- |
| 29 | `GET /` | `200` and `"ok"` | `200`, body `"ok"` | PASS | `test_read_root` |

## Known limitations

- The letters are plain text with no letterhead and no signature block; the
  customer name and account number are not placeholders, so they are not
  filled from the dispute record.
- `{deadline}` is taken from the dispute record as given. Nothing in this
  module computes a deadline from `{date}`, so the caller must supply one.
- The polish step is only exercised against stubbed responses here. A live
  call needs `ANTHROPIC_API_KEY` and is not part of the automated suite.
