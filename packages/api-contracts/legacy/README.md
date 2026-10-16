# Legacy contract (v0.1)

`openapi.v0.1.yaml` is the original scaffold contract. No backend ever served it: `apps/api` had no server when it was written, and `apps/web` does not call the API yet.

It is kept for reference while the frontend moves to the `/v1` contract in `../openapi.yaml`. It is not validated, not tested, and will be deleted once `apps/web/src/types/domain.ts` no longer refers to it.

Differences that matter to the frontend are listed in `docs/backend/contract-changes.md`.
