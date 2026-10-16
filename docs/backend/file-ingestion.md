# File-based ingestion (B2)

Sources that cannot call the event API can drop files. Everything here is configuration for synthetic data. The layouts are examples, not any real institution's or scheme's file format.

## How it works

1. A **mapping** (package `internal/feed`) describes one source's file: the format (`csv`, `fixedwidth`, `xml`), where each canonical field is, the source's event codes, the time layout **and its time zone**, and the amount unit (`minor` or `major` with at most two decimals). Mappings are stored by version in `feed_mappings`, never edited, and one version per name is active.
2. `POST /v1/feeds/{mapping}/files` (service assertion with `events:write`) takes the file as the body. The active mapping turns each row into the same envelope as `POST /v1/events`, and each row goes through the same `txn.Service.Ingest`, so the same validation, idempotency and conflict rules apply.
3. A row that cannot be read, or that ingestion refuses (invalid, contradicts its transaction, or reuses an event reference with different content), is **quarantined** with the mapping, version and line, and listed in the answer. The other rows are still imported.
4. The same bytes are imported once (`feed_files`, unique per mapping and content hash). A second upload returns the first result with `already_imported: true`. A different file that repeats events counts them as duplicates and stores nothing new. Rows are ingested one transaction each, so a crash part way is safe to repeat.

## Safety properties

- Unknown event codes are rejected, never guessed.
- Amounts are never rounded. More than two decimals in a major amount is rejected.
- A fixed-width line of the wrong length is rejected, because the columns may have shifted.
- XML with a DTD or entity declaration is refused. Files over 32 MiB or 100,000 rows are refused whole.
- Mapping and file records are tenant-scoped with row-level security.

## Limits

- There is no operator API to author or activate a mapping yet. Mappings are saved through `feed.Save` and `feed.Activate`. Authoring with maker-checker approval is not built.
- A file that is refused whole (unreadable, too large) is not recorded in `feed_files`; the caller gets the error.
- The message-consumer adapter, quarantine replay, correlation by external reference, source freshness and lag reporting are separate B2 units.
