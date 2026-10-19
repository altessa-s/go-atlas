# storagetest

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages/storagetest"
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
| `KeeperStoresReplayRecord` | A Keeper completes a key with a 256 KiB replay record and a repeat reads it back intact |

## Usage

```go
func TestStorageContract(t *testing.T) {
    t.Parallel()
    storagetest.Run(t, func(testing.TB) storages.Storage { return memory.New() })
}
```
