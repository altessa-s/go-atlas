# storagetest

```go
import "github.com/altessa-s/go-atlas/data/saga/storages/storagetest"
```

Contract suite for `saga.Storage` implementations. Every bundled backend runs it, and a custom backend should too: the orchestrator's correctness
rests on insert-if-absent creation, the version compare-and-swap of `Update` and the recovery predicate of `FetchRecoverable`.

## Contracts

| Function                     | Verifies                                                                                                                 |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| `Run`                        | Runs every contract below as parallel subtests, each on a fresh store; named contracts can be skipped                    |
| `RoundTrip`                  | Every field survives `Create`/`Get`, `LeaseUntil` at full precision; stored state is not aliased                         |
| `CreateExisting`             | `Create` on a stored ID fails with `ErrInstanceExists` and changes nothing                                               |
| `ConcurrentCreate`           | Exactly one of many concurrent creators of an ID wins                                                                    |
| `Update`                     | Version CAS: success persists and writes the version back; stale → `ErrVersionConflict`, missing → `ErrInstanceNotFound` |
| `ConcurrentUpdate`           | Exactly one of many concurrent updates from the same version wins                                                        |
| `Delete`                     | Removes the instance from `Get` and `FetchRecoverable`; a missing ID is not an error                                     |
| `Identity`                   | IDs differing only by case are distinct                                                                                  |
| `ExactIdentity`              | IDs compare byte for byte: trailing spaces, non-ASCII characters and quotes                                              |
| `FetchRecoverable`           | The predicate of `Instance.Recoverable`: active leases and terminal instances are excluded                               |
| `FetchRecoverableBoundaries` | A lease a nanosecond in the future is never returned; expired leases may be reported up to `Granularity` late            |
| `FetchRecoverableLimit`      | A positive limit caps the result; a non-positive one does not                                                            |

Fixtures use whole-second `CreatedAt`, `UpdatedAt`, `Deadline` and step times, which durable backends may store at one-second precision.
Versions are opaque: a contract checks only that a successful `Update` changes the version and writes it back.

## Usage

```go
func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) saga.Storage { return newStore(tb) })
}
```

A backend whose medium cannot satisfy a contract names it in `Run`'s variadic `skip` argument — the NATS backend skips `ExactIdentity`, since KV
keys admit only `[-/_=.A-Za-z0-9]`.
