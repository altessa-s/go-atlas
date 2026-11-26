# meilisearch

```go
import "github.com/altessa-s/go-atlas/store/meilisearch"
```

Thin wrapper around the [meilisearch-go](https://github.com/meilisearch/meilisearch-go) SDK that adds context propagation everywhere, structured
logging, sentinel-error classification, and an idempotent index-setup helper. Designed for use behind the configuration-driven
[factory/infrastructure/meilisearch](../../factory/infrastructure/meilisearch) builder, but the `Client` is usable standalone too.

## Quick Start

```go
client, err := meilisearch.New(ctx, "http://localhost:7700",
    meilisearch.WithAPIKey("masterKey"),
    meilisearch.WithLogger(logger),
)
if err != nil {
    return err
}
defer client.Close()

// Idempotent setup at startup
err = client.SetupIndexes(ctx, []meilisearch.IndexDefinition{
    {Name: "products", PrimaryKey: "id"},
    {Name: "users", PrimaryKey: "uid", Settings: &meilisearch.IndexSettings{
        SearchableAttributes: []string{"name", "email"},
        FilterableAttributes: []string{"role"},
    }},
})
```

## Options

| Option            | Default       | Description                                                                       |
|-------------------|---------------|-----------------------------------------------------------------------------------|
| `WithAPIKey`      | `""`          | Master or admin API key (Authorization header). Empty disables auth               |
| `WithTimeout`     | `30s`         | HTTP client timeout per request (incl. the synchronous startup health probe)      |
| `WithLogger`      | discard       | Structured logger                                                                 |
| `WithHTTPClient`  | nil           | Inject a fully configured `*http.Client` — typically used by the factory for TLS  |

`WithHTTPClient` takes precedence over `WithTimeout` (the caller passing a custom client has already expressed timeout intent).
Use a dedicated injected client when its connection pool must be isolated. A failed startup probe and `Client.Close` both close idle connections.

`New` and `Client.Health` require HTTP 200 with `{"status":"available"}` and read up to 64 KiB before decoding. Failures match `ErrHealthCheck`
and omit raw server and SDK error text. SDK retries are disabled. The client permits up to ten same-origin redirect hops, including with an
injected client. Origin comparison uses the scheme, canonical host with its trailing dot preserved, and effective port.

## Methods

`Client` implements `IndexManager`, `DocumentClient`, `Searcher`, `TaskWaiter`, and `HealthChecker`. Consumers can depend on the roles they use.

### Client lifecycle

| Method                         | Description                                                       |
|--------------------------------|-------------------------------------------------------------------|
| `New(ctx, host, opts...)`      | Creates a `Client` and synchronously probes `/health`             |
| `Client.Health(ctx)`           | Pings the server                                                  |
| `Client.Close()`               | Releases idle HTTP connections                                    |

### Indexes

| Method                                                        | Description                                                 |
|---------------------------------------------------------------|-------------------------------------------------------------|
| `Client.IndexExists(ctx, name)`                               | `(exists bool, err error)` — `false, nil` on index_not_found |
| `Client.EnsureIndex(ctx, name, primaryKey, settings)`         | Idempotent create-or-update; waits for the create and settings tasks |
| `Client.UpdateIndexSettings(ctx, name, settings)`             | Update searchable / filterable / sortable attrs; waits for the task |
| `Client.SetupIndexes(ctx, defs)`                              | Bulk idempotent EnsureIndex over a slice of definitions      |
| `Client.SwapIndexes(ctx, pairs...)`                           | Atomically swap document sets of `SwapPair`s in one task; returns task UID (errors on zero pairs) |
| `Client.DeleteIndex(ctx, name)`                               | Delete an index; returns task UID, or `0, nil` when it is already absent |

### Documents

| Method                                                            | Description                                                |
|-------------------------------------------------------------------|------------------------------------------------------------|
| `Client.IndexDocuments(ctx, index, docs)`                         | Add or update documents; returns task UID                  |
| `Client.DeleteDocument(ctx, index, id)`                           | Delete single document; returns task UID                   |
| `Client.DeleteDocuments(ctx, index, ids)`                         | Delete by ID list                                          |
| `Client.DeleteDocumentsByFilter(ctx, index, filter)`              | Delete by Meilisearch filter expression (see SECURITY)     |
| `Client.FetchDocuments(ctx, index, filter, offset, limit)`        | Paginated fetch by filter; returns `*FetchResult` with raw hits and total (see SECURITY) |
| `Client.GetAllDocumentIDs(ctx, index)`                            | Paginated ID list (default `"id"` primary key; string and numeric IDs) |
| `Client.GetAllDocumentIDsWithPrimaryKey(ctx, index, key)`         | Same, but with an explicit primary-key field name          |
| `Client.Search(ctx, req)`                                         | Full-text search; returns `*SearchResult` with raw JSON hits and estimated total |

`GetAllDocumentIDs` and `GetAllDocumentIDsWithPrimaryKey` decode both string and numeric (`json.Number`) primary keys and fail with
`ErrInvalidDocumentID` when a document lacks a valid scalar primary key. Offset paging over a live, mutating index is best-effort: a delete and
insert between pages can shift offsets and skip a row even when `total` is unchanged. Run against a quiescent index or reconcile twice.

### Tasks

Write methods (`IndexDocuments`, `DeleteDocuments`, `SwapIndexes`, …) are fire-and-forget: they return a Meilisearch task UID. Await completion with
`WaitForTask`.

| Method                                          | Description                                                                                  |
|-------------------------------------------------|----------------------------------------------------------------------------------------------|
| `Client.WaitForTask(ctx, taskUID, interval)`    | Block until the task reaches a terminal state (`interval` 0 = SDK default 50ms); wraps `ErrTaskFailed` on a non-`succeeded` status |

## Errors

**Breaking change.** Sanitized diagnostics unwrap to package sentinels such as `ErrTaskFailed` or `ErrRequestFailed`. The error chain no longer
exposes `*meilisearch.Error` from the SDK, so callers using `errors.As` for SDK fields must switch to the wrapper's predicates and sentinels.

| Sentinel                 | Predicate                          | Maps to Meilisearch code   |
|--------------------------|------------------------------------|----------------------------|
| `ErrIndexNotFound`       | `IsErrorIndexNotFound(err)`        | `index_not_found`          |
| `ErrIndexAlreadyExists`  | `IsErrorIndexAlreadyExists(err)`   | `index_already_exists`     |
| `ErrTaskFailed`          | `errors.Is(err, ErrTaskFailed)`    | task terminal status ≠ `succeeded` |
| `ErrInvalidDocumentID`   | `errors.Is(err, ErrInvalidDocumentID)` | missing, null, or non-scalar primary key |

Use the predicates to classify API errors: they match the sentinel or the validated API error code. Returned errors render only the operation,
task UID, status, HTTP status, and validated Meilisearch `code`/`type` — raw server `message` text and request/response bodies are omitted so
document values and search filters cannot leak into logs.

## Security

### Filter expressions

`Client.DeleteDocumentsByFilter` and `SearchRequest.Filter` pass the filter string through to Meilisearch unchanged. Meilisearch filters are a DSL, not
SQL, but they still support field comparisons and boolean composition — building a filter from concatenated user input lets an attacker widen the scope
(e.g. `tenant_id = 'their_id' OR true` deletes the whole index). Construct filters via a trusted DSL builder, escape values, or restrict the caller's
role at the API-key level.

### PII in logs

`Client.Search` and `Client.FetchDocuments` do NOT log the query / filter — they can carry email addresses, names, or other PII. Use Meilisearch's
server-side request logging if you need them for debugging.

`Client.DeleteDocumentsByFilter` does log the filter at Debug level (operator-visible). Treat the Debug log as containing whatever values the filter
encodes.

## TLS

The package does not parse TLS settings itself. Build an `*http.Client` with the desired `*tls.Config` and pass it via `WithHTTPClient`. The
[factory/infrastructure/meilisearch](../../factory/infrastructure/meilisearch) builder threads `tlsconfig.Client` through automatically.

## See also

- [factory/infrastructure/meilisearch](../../factory/infrastructure/meilisearch) — configuration-driven builder, TLS support, health-coordinator hook
- [meilisearchconfig.Config](../../config/meilisearch/meilisearch.go) — YAML-facing config struct
