# mongo

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/mongo"
```

MongoDB provider for [dlock](../../README.md): each lock key is one lease document, taken by a single atomic upsert, renewed in the
background and released when the lock's context ends. Lease expiry is judged by the MongoDB server clock, and every acquisition increments a
fencing token.

## Options

| Option                  | Default  | Description                                                                 |
|-------------------------|----------|-----------------------------------------------------------------------------|
| `WithCollection`        | `dlocks` | Collection holding one lease document per key                               |
| `WithTTL`               | 10s      | Lease length, truncated to whole milliseconds                               |
| `WithRenewRatio`        | 1/3      | Fraction of the TTL after which the lease is renewed; TTL × ratio ≥ 1ms, < TTL |
| `WithOperationsTimeout` | 5s       | Bound of one acquisition attempt, renewal or release                        |
| `WithLogger`            | discard  | Logger for renewal and release failures                                     |

## Semantics

| Aspect          | Behavior                                                                                                    |
|-----------------|-------------------------------------------------------------------------------------------------------------|
| Acquire         | One attempt; `errs.ErrLockNotHeld` while another holder has an unexpired lease                              |
| Clock           | `$$NOW` of the server decides expiry, so client clock skew does not matter                                  |
| Fencing token   | +1 per acquisition of the key; documents are kept after release, so tokens never restart                   |
| Renewal         | Every TTL × renew ratio while the lock's context lives; a failed round trip is retried on the next tick     |
| Lost lease      | A lease that expired (taken over or not) is never renewed again; a new acquisition gets a new token        |
| Late acquire    | A reply arriving after the TTL counted from the request is released and reported as `ErrLockNotHeld`     |
| Ambiguous error | An acquisition that errored may still have applied: it is released by its unique owner id (Close retries) |
| Release         | Matches holder and fencing token: a stale holder cannot release the new holder's lease                     |
| Context end     | Releases the lease                                                                                          |
| Durability      | Lock writes use majority write concern regardless of the database handle, so a failover cannot reissue a token |
| Release failure | A failed release keeps the lease and stays retryable through `Release`; `Close` returns release failures     |
| Close           | Stops all renewals, waits for acquisitions in flight, releases every held lock; a repeated Close retries   |
| Unconfirmed     | No renewal confirmed within the lease: renewing stops and the lease is released conditionally              |
| Reads           | `GetLockInfo` reads the primary regardless of the client's read preference, so it sees this client's own writes |
| Sessions        | Lock operations never join a session or transaction carried by the caller's context (only its cancellation and deadline apply) |
| Indexes         | None besides `_id`                                                                                          |

The collection keeps one small document per key ever locked, and that document is the key's fencing history: deleting it restarts the key's
tokens at 1, so a stale holder could carry a higher token than the new one. Delete documents only of keys that are permanently retired and
have no outstanding holder; keep them for every key that may be locked again.
