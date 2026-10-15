# Sample debit alerts

Invented debit alerts for parser development. They contain **no real customer
data** - every account number, reference and beneficiary is made up.

Hand these to the parsing workstream first thing: each file holds one raw alert
exactly as a customer would paste it. Files ending in `.txt` are the alerts;
this file is an index only.

## Index

| File | Bank | Channel | Fields present |
| --- | --- | --- | --- |
| `alert_01_gtbank_pos.txt` | GTBank | pos | amount, date, time, channel, bank, reference |
| `alert_02_access_atm.txt` | Access Bank | atm | amount, date, time, channel, bank, reference |
| `alert_03_zenith_transfer.txt` | Zenith Bank | transfer | amount, date, time, channel, bank, reference |
| `alert_04_firstbank_web.txt` | FirstBank | web | amount, date, time, channel, bank, reference |
| `alert_05_uba_pos.txt` | UBA | pos | amount, date, time, channel, bank, reference |
| `alert_06_fidelity_atm.txt` | Fidelity Bank | atm | amount, date, time, channel, bank, reference |
| `alert_07_sterling_transfer.txt` | Sterling Bank | transfer | amount, date, time, channel, bank, reference, beneficiary |
| `alert_08_ecobank_web.txt` | Ecobank | web | amount, date, time, channel, bank, reference |
| `alert_09_gtbank_atm.txt` | GTBank | atm | amount, date, time, channel, bank, reference |
| `alert_10_access_transfer.txt` | Access Bank | transfer | amount, date, time, channel, bank, reference, beneficiary |
| `alert_11_zenith_pos.txt` | Zenith Bank | pos | amount, date, time, channel, bank, reference, merchant |
| `alert_12_wema_web.txt` | Wema Bank | web | amount, date, time, channel, bank, reference |
| `alert_13_fcmb_pos.txt` | FCMB | pos | amount, date, time, channel, bank, reference |

## Notes for the parser

- Formats vary on purpose: single-line SMS, key/value blocks, `NGN` and `₦`.
- Dates appear as `DD-MM-YYYY`, `DD/MM/YYYY`, `YYYY-MM-DD` and `DD-Mon-YYYY`.
- `channel` should normalise to one of `atm`, `pos`, `transfer`, `web`.
- An account number and a balance appear in most alerts; neither is a
  required field.
