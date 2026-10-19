# keyset

```go
import "github.com/altessa-s/go-atlas/data/keyset"
```

Package `keyset` issues and resolves signed, stateless page tokens for keyset (cursor) pagination. A token carries an opaque position payload, its issue
time and digests of its bindings — sort, filter and subject fingerprints — signed with HMAC-SHA256.

## Usage

```go
codec, err := keyset.New(signingKey) // ≥ 32 bytes, identical on every replica
token, err := codec.Issue([]byte(position), keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
position, err := codec.Resolve(token, keyset.Bindings{Sort: sortFP, Filter: filterFP, Subject: user})
```

`Resolve` verifies the signature first, then expiry, then each binding, so a forged token never reveals which binding it would have failed. Tokens use
canonical URL-safe base64 only; any other spelling of the same bytes is rejected. Payloads are limited to `MaxPayloadLength` bytes and tokens to the
matching encoded length. Tokens are stateless: they cannot be revoked individually, only by rotating the key.

## Typed cursors and offsets

```go
tokens := keyset.NewTyped[Position](codec) // JSON-encoded cursor; nil codec = single page
b := keyset.NewBindings("ListItems", "name:1", filter, kind) // filter parts are length-prefixed
token, err := tokens.Issue(Position{Name: last.Name, ID: last.ID}, b)
after, first, err := tokens.Resolve(req.PageToken, b) // "" → first page
offset, err := keyset.NewOffset(codec).Resolve(req.PageToken, b) // small in-memory catalogs
```

`Typed[C]` and `Offset` over a nil codec issue no tokens (a single page) and fail a presented token with `ErrCodecRequired`. A verified payload that
does not decode, or a negative offset, is `ErrInvalidToken`.

## Slice pagers and page sizes

```go
size := keyset.PageSize(req.GetPageSize(), 50, 200) // ≤ 0 → 50, clamped to [1, 200]
pager := keyset.NewSlicePager(keyset.NewTyped[string](codec), func(it Item) string { return it.Name }, strings.Compare)
page, err := pager.Page(items, size, req.GetPageToken(), keyset.NewBindings("ListItems", "name:1"))
// page.Items, page.Next
```

| Symbol                         | Description                                                                                              |
|--------------------------------|----------------------------------------------------------------------------------------------------------|
| `PageSize(req, def, max)`      | Zero or negative is `def`, the result is clamped to `[1, max]` (`max < 1` is 1); a caller that rejects a negative size checks it first |
| `NewSlicePager(t, key, cmp)`   | Keyset pager over a slice ordered by `cmp` on `key`: changes between pages neither repeat nor skip items |
| `NewSliceOffsetPager(o)`       | Offset pager: the token holds the index of the next page; an offset beyond the slice is `ErrInvalidToken` |
| `SlicePager.Page`              | Returns `SlicePage{Items, Next}`; `Next` is empty on the last page and without a codec                   |

A slice pager over a nil codec serves a single page and fails a presented token with `ErrCodecRequired`. `PageSize` is the clamp `data/audit`,
`data/mongo` (`FindPage`) and `service/scheduler` apply with their own defaults and maximums.

## Error classification

`Classify(err)` reports a page-token failure independently of the transport: `FailureMalformed` (invalid, tampered, foreign, decoded badly, bound to
another sort, filter or subject, or presented without a codec) or `FailureExpired`; any other error, including `ErrPayloadTooLarge` and
`ErrWeakKey`, is `FailureNone`. `Failure.String()` is the reason a transport reports (`malformed`, `expired`) in a field violation on the
page-token field.

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
| `ErrCodecRequired`   | Token presented to a `Typed`/`Offset` (or `FindPage`) without a codec |
| `ErrNegativeOffset`  | `Offset.Issue` called with a negative offset (caller bug)     |
