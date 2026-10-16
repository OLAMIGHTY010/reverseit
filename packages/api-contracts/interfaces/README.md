# External interface contracts

One OpenAPI file per interface in scope section 13.3. Each describes what the SMARTREVERSAL engine expects from, or offers to, an outside system. The Track H mock services implement these files. A real integration replaces a mock by implementing the same file, so the engine does not change.

These are our contracts, written from the scope and the v2 prompt pack. They are not copies of any bank, NIBSS or provider specification. Real status codes, field names and file layouts must be taken from the institution's rail documents during discovery. Wherever the playbooks say "VERIFY", the value here is illustrative and marked `x-verify: true`.

| File | Direction | Track H implementation |
|---|---|---|
| `transaction-feed.yaml` | In | Simulator publishing events |
| `status-query.yaml` | Out | Mock status service with scripted outcomes |
| `reversal-api.yaml` | Out | Mock reversal service |
| `settlement-files.yaml` | In | Generated files |
| `notification-gateway.yaml` | Out | Mock notification service |
| `customer-app.yaml` | In and out | ReverseIt API |

Every mock response carries `"simulated": true`.
