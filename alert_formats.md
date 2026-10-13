# Invented Alert Formats for Testing

## 1. GTBank
Alert: "GTBank: Your account ending in 4521 has been debited NGN15,000.00 on 13/10/2026 at 09:15 via ATM. Ref: GTB20261013001."
Fields:
- amount: 15000.0
- date: 2026-10-13
- time: 09:15
- channel: atm
- bank: GTBank
- reference: GTB20261013001

## 2. First Bank
Alert: "FirstBank Debit Alert: NGN8,500.50 was withdrawn from your account ******6789 via POS on 2026-10-12 16:30 at SUPER MART. Reference: FBK123456789."
Fields:
- amount: 8500.5
- date: 2026-10-12
- time: 16:30
- channel: pos
- bank: First Bank
- reference: FBK123456789

## 3. Access Bank
Alert: "AccessBank Notification: Transfer of NGN3,200.00 to account 0123456789 completed on 12-Oct-2026 at 10:30. Reference: ACC20261012TRF."
Fields:
- amount: 3200.0
- date: 2026-10-12
- time: 10:30
- channel: transfer
- bank: Access Bank
- reference: ACC20261012TRF

## 4. Zenith Bank
Alert: "Zenith Bank Alert: Your card ending in 1234 was used for a web payment of NGN12,400.00 on 10/10/2026 at 22:45. Transaction ID: ZBWEB20261010001."
Fields:
- amount: 12400.0
- date: 2026-10-10
- time: 22:45
- channel: web
- bank: Zenith Bank
- reference: ZBWEB20261010001

## 5. UBA
Alert: "UBA Mobile Money: NGN500.00 sent to ******1122 via Transfer on 13-10-2026 14:05. Ref: UBAUSSD131026."
Fields:
- amount: 500.0
- date: 2026-10-13
- time: 14:05
- channel: transfer
- bank: UBA
- reference: UBAUSSD131026
