# keyset

```go
import "github.com/altessa-s/go-atlas/data/keyset"
```

Package `keyset` issues and resolves signed, stateless page tokens for keyset (cursor) pagination. A token carries an opaque position
payload, its issue time and digests of its bindings — sort, filter and subject fingerprints — signed with HMAC-SHA256.

## Usage

```go
codec, err := keyset.New(signingKey) // ≥ 32 bytes, identical on every replica
token, err := codec.Issue([]byte(position), keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
position, err := codec.Resolve(token, keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
```

`Resolve` verifies the signature first, then expiry, then each binding, so a forged token never reveals which binding it would have failed.
Tokens use canonical URL-safe base64 only; any other spelling of the same bytes is rejected. Payloads are limited to `MaxPayloadLength`
bytes and tokens to the matching encoded length. Tokens are stateless: they cannot be revoked individually, only by rotating the key.

## Options

| Option               | Default              | Description                                                   |
|----------------------|----------------------|---------------------------------------------------------------|
| `WithTTL`            | `DefaultTTL` (24h)   | How long an issued token stays valid                          |
| `WithExpiryDisabled` | off                  | Tokens never expire                                           |
| `WithPreviousKeys`   | none                 | Keys that still verify tokens signed before a rotation        |
| `WithMaxClockSkew`   | `DefaultMaxClockSkew` (1m) | How far in the future an issue time may lie            |
| `WithClock`          | `time.Now`           | Time source, for tests                                        |

## Errors

| Error                | Meaning                                                       |
|----------------------|---------------------------------------------------------------|
| `ErrInvalidToken`    | Malformed token, bad signature or issue time in the future    |
| `ErrExpiredToken`    | Token older than the TTL                                      |
| `ErrSortChanged`     | Query sort differs from the token's                           |
| `ErrFilterChanged`   | Query filter differs from the token's                         |
| `ErrSubjectMismatch` | Token issued to another principal                             |
| `ErrPayloadTooLarge` | `Issue` payload over `MaxPayloadLength`                       |
| `ErrWeakKey`         | Signing key shorter than `MinKeyLength`                       |
