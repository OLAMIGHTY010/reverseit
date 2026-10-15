# Alert Samples for ReverseIt

## Sample 1
**Alert:** "Debit NGN200.00 from ExampleBank on 13/10/2026 at 14:05 via POS. Ref:12345"
**Expected Fields:**
- amount: 200.0
- date: 2026-10-13
- time: 14:05
- channel: pos
- bank: ExampleBank
- reference: 12345

## Sample 2
**Alert:** "Purchase of ₦500.50 made on 2026-10-12 09:30:00 through WEB at AnotherBank. Ref: ABC98765"
**Expected Fields:**
- amount: 500.5
- date: 2026-10-12
- time: 09:30
- channel: web
- bank: AnotherBank
- reference: ABC98765

## Sample 3
**Alert:** "ATM withdrawal: NGN1,500.00 on 12-Oct-2026 15:30 from ThirdBank. Transaction ID: TX1234"
**Expected Fields:**
- amount: 1500.0
- date: 2026-10-12
- time: 15:30
- channel: atm
- bank: ThirdBank
- reference: TX1234

## Sample 4
**Alert:** "Transfer sent: NGN3,000 on 13/10/2026 at 16:45 via Transfer from FourthBank to John Doe. Ref: TRF7890"
**Expected Fields:**
- amount: 3000.0
- date: 2026-10-13
- time: 16:45
- channel: transfer
- bank: FourthBank
- reference: TRF7890

## Sample 5
**Alert:** "Web payment of ₦1234.56 successful on 2026-10-13 11:22:33 with FifthBank. ID: XYZ123"
**Expected Fields:**
- amount: 1234.56
- date: 2026-10-13
- time: 11:22
- channel: web
- bank: FifthBank
- reference: XYZ123