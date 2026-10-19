# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
```

Package `idempotency` provides middleware for idempotent HTTP request handling. Prevents duplicate processing of non-safe methods (POST, PUT, PATCH,
DELETE) via the Idempotency-Key header (lowercase UUID v4). The middleware acquires a lock in the configured storage before the handler runs; on success
the key is marked complete, on failure it is deleted so the client can retry. Safe methods (GET, HEAD, OPTIONS) bypass the check. Supports configurable
fallback behavior when storage is unavailable.

## Options

| Option                            | Default                     | Description                                                   |
|-----------------------------------|-----------------------------|---------------------------------------------------------------|
| `WithIdempotencyKeyHeader`        | `Idempotency-Key`           | Request header carrying the key                               |
| `WithIdempotencyKeyStatusHeader`  | `Idempotency-Key-Status`    | Response header with the key status                           |
| `WithIdempotencyKeyEntityIDHeader`| `Idempotency-Key-Entity-Id` | Response header with the entity ID                            |
| `WithFallbackBehavior`            | `fallback.Deny`             | Behavior on storage failure                                   |
| `WithEnforceMandatory`            | `false`                     | Require a key on every non-safe request                       |
| `WithKeyFormatValidator`          | `DefaultKeyValidator`       | Key format check (lowercase UUID v4); `OpaqueKeyValidator(n)` / `PatternKeyValidator(re, n)` accept non-UUID keys |
| `WithKeyQueryParam`               | "" (header only)            | Query parameter carrying the key (such as `request_id`); wins over the header |
| `WithRequestClassifier`           | nil                         | Per-request `Policy` (skip, optional, mandatory, default)     |
| `WithResponseReplay`              | off                         | Write the stored response to a repeat of a completed request (see below) |
| `WithMaxReplaySize`               | `DefaultMaxReplaySize` (256 KiB) | Largest stored body (at most `MaxCaptureBodySize`, 1 MiB) |
| `WithReplayHeaders`               | `DefaultReplayHeaders`      | Response headers stored and replayed                          |
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

## Key sources and request policies

The key is read from the query parameter named by `WithKeyQueryParam` when it holds a non-empty value (such as `request_id`),
otherwise from the key header; it must pass the `KeyFormatValidator` (lowercase UUID v4 by default, `OpaqueKeyValidator` or `PatternKeyValidator`
for client request IDs). A `RequestClassifier`, run after the ignore lists and the safe-method skip, returns a `Policy` per request: `PolicySkip`,
`PolicyOptional`, `PolicyMandatory` or `PolicyDefault` (`WithEnforceMandatory` decides). In YAML: `keyQueryParam`, `keyFormat`, `keyPattern`,
`maxKeyLength`, `mandatoryPaths` and `mandatoryPatterns`.

## Response replay

With `WithResponseReplay()` (YAML `responseReplay: true`) a completed 2xx response stores its status, the `WithReplayHeaders` headers
(`DefaultReplayHeaders`: Content-Type, Content-Encoding, Content-Language, Content-Disposition, Location, ETag, Last-Modified) and its body, when the
whole body was captured and fits `WithMaxReplaySize`. A repeat of the request receives exactly that response (plus `Idempotency-Key-Status:
success`) without running the handler, instead of `422`; cookies, request IDs and other per-request headers are never replayed. In-flight repeats
stay `409`, failed responses store nothing, and the fingerprint and owner checks run first; the record is bound to the request method and path,
and only a request with the same binding receives it. A larger body, a record written before replay or without a binding, and a truncated
capture keep the `422` answer. Replay is off by default because the repeat's caller receives the original response: enable it with keys
scoped per principal (`idempotency.WithOwner`).

## Storage keys

The middleware passes the Keeper the tuple `("transport", "{METHOD}:/{path}", key)` in the length-prefixed v2 format of
[`data/idempotency.ComposeKey`](../../../../../data/idempotency#storage-key-format) — `POST /api/v1/users` with key `550e8400-…` becomes
`idk:v2:9:transport18:POST:/api/v1/users36:550e8400-…` — and the Keeper composes `("namespace", namespace, key)` around it before storage, so a `:` in
the path or the key cannot make two requests share a record. The format replaced `idk:{METHOD}:/{path}:{key}` with a clean cut: follow the
[rollout contract](../../../../../data/idempotency#rolling-out-the-v2-key-format) when upgrading.

## Request fingerprints and owners

Each key records a fingerprint of the method, path, query and whole body. The body is hashed while it is spooled — up to `WithFingerprintMemThreshold`
(1 MiB) in memory, the rest in a temp file under `WithFingerprintTempDir` — and handed to the handler unchanged; a body over `WithFingerprintMaxBody`
(32 MiB) gets 413 and one that fails to read gets 400, before any key is locked. A key reused for a different request gets 422 instead of being treated
as a replay; `WithRequestFingerprint(false)` (YAML `disableRequestFingerprint: true`) turns this off. When the Keeper records owners
(`idempotency.WithOwner`), a key of another caller gets 403. A `ReplayAuthorizer` refusal that carries an HTTP status (`HTTPStatus() int`, as
`writer.HTTPStatuser` errors do) is answered with that status instead — return a 404 error to hide the record.

The stored entity ID is echoed to a duplicate request only with `WithExposeEntityID()` (YAML `exposeEntityID: true`): the caller presenting a duplicate
key is not verified to be the entity's creator unless the Keeper checks owners.

## Security

Keys are caller-chosen and may embed user data, and the invalid-format branch sees values that failed validation. `WithKeyLogMode` controls how the key
reaches debug-level logs.

| Mode           | Value    | Logged attribute                               |
|----------------|----------|------------------------------------------------|
| `KeyLogHashed` | `hashed` | `key_hash` — 16 hex chars of SHA-256 (default) |
| `KeyLogFull`   | `full`   | `key` — the raw key                            |
| `KeyLogOff`    | `off`    | none                                           |

Hashing is log hygiene rather than a privacy guarantee — a key drawn from a small or guessable set can still be recovered by hashing candidates. For
end-to-end redaction, run production loggers at Info or above, or wrap the handler with
[`observability/slog/handler/masking`](../../../../../observability/slog/handler/masking/README.md).
