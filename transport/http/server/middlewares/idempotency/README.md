# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
```

Package `idempotency` provides middleware for idempotent HTTP request handling. Prevents duplicate processing of non-safe methods (POST,
PUT, PATCH, DELETE) via the Idempotency-Key header (lowercase UUID v4). The middleware acquires a lock in the configured storage before
the handler runs; on success the key is marked complete, on failure it is deleted so the client can retry. Safe methods (GET, HEAD,
OPTIONS) bypass the check. Supports configurable fallback behavior when storage is unavailable.

## Options

| Option                            | Default                     | Description                                                   |
|-----------------------------------|-----------------------------|---------------------------------------------------------------|
| `WithIdempotencyKeyHeader`        | `Idempotency-Key`           | Request header carrying the key                               |
| `WithIdempotencyKeyStatusHeader`  | `Idempotency-Key-Status`    | Response header with the key status                           |
| `WithIdempotencyKeyEntityIDHeader`| `Idempotency-Key-Entity-Id` | Response header with the entity ID                            |
| `WithFallbackBehavior`            | `fallback.Deny`             | Behavior on storage failure                                   |
| `WithEnforceMandatory`            | `false`                     | Require a key on every non-safe request                       |
| `WithKeyFormatValidator`          | `DefaultKeyValidator`       | Key format check (lowercase UUID v4)                          |
| `WithKeyLogMode`                  | `KeyLogHashed`              | How the key reaches debug logs (see Security)                 |
| `WithEntityIDExtractor`           | nil                         | Extracts the entity ID from the response                      |
| `WithExposeEntityID`              | off                         | Echo the stored entity ID to a duplicate request              |
| `WithRequestFingerprint`          | `true`                      | Refuse a key reused for a different request                   |
| `WithFingerprintMaxBody`          | `DefaultFingerprintMaxBody` (32 MiB) | Largest body read for the fingerprint                |
| `WithFingerprintMemThreshold`     | `DefaultFingerprintMemThreshold` (1 MiB) | Part of the body kept in memory while hashing    |
| `WithFingerprintTempDir`          | system temp dir             | Spool directory for the rest of the body                      |
| `WithErrorHandler`                | plain-text responses        | Writes the response for each `ErrorScenario`                  |
| `WithIgnorePaths`                 | --                          | Paths to skip                                                 |
| `WithIgnorePatterns`              | health, metrics, pprof      | Regex patterns for paths to skip                              |
| `WithLogger`                      | discard                     | Structured logger                                             |

## Error scenarios

| Scenario                 | Default status |
|--------------------------|----------------|
| `ErrorIDKMissing`        | 400            |
| `ErrorIDKInvalidFormat`  | 400            |
| `ErrorIDKInProgress`     | 409            |
| `ErrorIDKAlreadyUsed`    | 422            |
| `ErrorIDKMismatch`       | 422            |
| `ErrorIDKForbidden`      | 403            |
| `ErrorIDKBodyTooLarge`   | 413            |
| `ErrorIDKBodyUnreadable` | 400            |

## Request fingerprints and owners

Each key records a fingerprint of the method, path, query and whole body. The body is hashed while it is spooled — up to
`WithFingerprintMemThreshold` (1 MiB) in memory, the rest in a temp file under `WithFingerprintTempDir` — and handed to the handler unchanged; a
body over `WithFingerprintMaxBody` (32 MiB) gets 413 and one that fails to read gets 400, before any key is locked. A key reused for a different
request gets 422 instead of being treated as a replay; `WithRequestFingerprint(false)` (YAML `disableRequestFingerprint: true`) turns this off.
When the Keeper records owners (`idempotency.WithOwner`), a key of another caller gets 403. A `ReplayAuthorizer` refusal that carries an
HTTP status (`HTTPStatus() int`, as `writer.HTTPStatuser` errors do) is answered with that status instead — return a 404 error to hide the
record.

The stored entity ID is echoed to a duplicate request only with `WithExposeEntityID()` (YAML `exposeEntityID: true`): the caller presenting
a duplicate key is not verified to be the entity's creator unless the Keeper checks owners.

## Security

Keys are caller-chosen and may embed user data, and the invalid-format branch sees values that failed validation. `WithKeyLogMode`
controls how the key reaches debug-level logs.

| Mode           | Value    | Logged attribute                               |
|----------------|----------|------------------------------------------------|
| `KeyLogHashed` | `hashed` | `key_hash` — 16 hex chars of SHA-256 (default) |
| `KeyLogFull`   | `full`   | `key` — the raw key                            |
| `KeyLogOff`    | `off`    | none                                           |

Hashing is log hygiene rather than a privacy guarantee — a key drawn from a small or guessable set can still be recovered by hashing
candidates. For end-to-end redaction, run production loggers at Info or above, or wrap the handler with
[`observability/slog/handler/masking`](../../../../../observability/slog/handler/masking/README.md).
