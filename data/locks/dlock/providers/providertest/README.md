# providertest

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/providertest"
```

Contract suite for `providers.Provider` implementations. Every bundled lock backend runs it, and a custom one should too. A `Backend` is one
isolated namespace from which several providers contend for the same keys, like replicas of one service.

## Contracts

| Function                   | Verifies                                                                                          |
|----------------------------|---------------------------------------------------------------------------------------------------|
| `Run`                      | Runs every contract below as parallel subtests, each on a fresh backend; named ones can be skipped |
| `Exclusive`                | A held key cannot be taken again, through another provider or the same one                        |
| `ConcurrentLock`           | Exactly one of many concurrent acquisitions across providers wins; the rest get `ErrLockNotHeld`  |
| `ReleaseHandsOver`         | Release frees the key at once with a higher fencing token next; a second Release is a no-op       |
| `FencingMonotonic`         | The fencing token of a key strictly grows across acquisitions                                     |
| `LockInfo`                 | Provider and lock report the same lease; a free key has none                                      |
| `Renewal`                  | A held lock outlives several TTLs and stays exclusive                                             |
| `ContextEndReleases`       | Ending the lock's context releases it                                                             |
| `FailedReleaseIsRetryable` | A failed release keeps the lease; a later one succeeds                                            |
| `CloseReleasesAndRejects`  | Close releases every held lock and rejects further acquisitions                                   |
| `CloseRacingLock`          | No lock acquired while Close runs is left held                                                    |
| `ExpiredLeaseIsTakenOver`  | An expired lease is taken over with a higher token; the stale holder cannot see or release it     |
| `ExpiredLeaseStaysLost`    | Renewal does not revive an expired lease                                                          |

The last two need `Backend.Expire`, which ages a lease out from under its holder, and are skipped without it. Providers should use a TTL of
about a second: `Renewal` holds a lock for 2.5 TTLs.

## Usage

```go
func TestProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(tb testing.TB) providertest.Backend {
		ns := newNamespace(tb)
		return providertest.Backend{NewProvider: ns.provider, Expire: ns.expire}
	})
}
```

The NATS provider skips `FailedReleaseIsRetryable` and `CloseRacingLock`, which it does not pass yet.
