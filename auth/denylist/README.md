# denylist

```go
import "github.com/altessa-s/go-atlas/auth/denylist"
```

Reusable token-revocation seam: a concurrency-safe set of revoked identifiers (a JWT ID/`jti`, or a subject for a blanket ban) that a
verifier consults to reject tokens revoked before their natural expiry. Transport- and JWT-free, keyed by a plain string, so any token
package (`auth/jwt`, `auth/oidc`, `selfjwt`) can adopt it.

## Types

| Type        | Description                                                                                     |
|-------------|-------------------------------------------------------------------------------------------------|
| `Denylist`  | Concurrency-safe revoked-identifier set with permanent and expiry-bounded entries.              |
| `Checker`   | Read seam (`IsRevoked(key) bool`) a verifier accepts so the backing store can be swapped later. |

## Methods

| Method                       | Description                                                                        |
|------------------------------|------------------------------------------------------------------------------------|
| `Revoke(key)`                | Deny `key` permanently, until a matching `Restore`.                                |
| `RevokeUntil(key, expiry)`   | Deny `key` until `expiry`, then forget it. An expiry at or before now is a no-op.  |
| `Restore(key)`               | Re-allow `key`.                                                                    |
| `IsRevoked(key) bool`        | Whether `key` is currently revoked (expired entries report false).                |
| `Sweep() int`                | Drop expired entries and report the count. Writes sweep automatically.             |
| `Len() int`                  | Tracked entries, including not-yet-swept expired ones — for observability/tests.   |

## Options

| Option            | Description                                                  |
|-------------------|------------------------------------------------------------|
| `WithClock(now)`  | Override the time source (tests); production uses `time.Now`. |

## Bounding

Expired entries are swept on every write and ignored on reads, so a denylist that only revokes until each token's own expiry stays bounded
without a background goroutine. Unlike a cache it **never** evicts a live (unexpired) entry — that would silently un-revoke a token — so
growth is bounded by TTL expiry and the caller's use of permanent revocations, never by capacity.

## Usage

```go
dl := denylist.New()
dl.RevokeUntil(claims.ID(), claims.ExpiresAt()) // deny this token until it expires anyway

if dl.IsRevoked(claims.ID()) {
    return ErrRevoked
}
```

See also [`auth/mtls`](../mtls) `RevocationList` — the certificate-serial analogue for mTLS peers.
