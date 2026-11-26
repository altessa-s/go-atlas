# storagetest

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/storagetest"
```

Conformance kit for [`storages.Storage`](../interface.go) backends. `Run` executes every contract as a parallel subtest on a fresh store from the
caller's constructor, which must return an empty store isolated from every other store it returns.

## Key types

| Symbol                      | Description                                                     |
|-----------------------------|-----------------------------------------------------------------|
| `Run(t, newStore, skip...)` | Runs all contracts; the names in `skip` are reported as skipped |

## Contracts

| Contract                   | Checks                                                                                  |
|----------------------------|-----------------------------------------------------------------------------------------|
| `KeeperStoresReplayRecord`   | A Keeper completes a key with a 256 KiB replay record and a repeat reads it back intact |
| `StealRefusesCompletedRecord` | Storage refuses matching completed values and preserves the record                     |

## Usage

```go
func TestStorageContract(t *testing.T) {
    t.Parallel()
    storagetest.Run(t, func(tb testing.TB) storages.Storage {
        store, err := memory.New()
        require.NoError(tb, err)
        return store
    })
}
```

`KeeperUsesStorageClock` checks that a fresh lock survives an apparently old
wire timestamp and that takeover refuses a mismatched expected value. Run it
with stores that consult a backend clock.
