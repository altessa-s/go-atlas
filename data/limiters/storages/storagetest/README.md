# storagetest

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages/storagetest"
```

Conformance kit for [`storages.MultiStorage`](../multi.go) backends. `Run` executes every contract as a parallel subtest on a fresh store from the
caller's constructor, which must return an empty store isolated from every other store it returns. The keys of one call share the hash tag `{t}`, so
the kit also runs against a Redis Cluster backend.

## Key types

| Symbol                      | Description                                                     |
|-----------------------------|-----------------------------------------------------------------|
| `Run(t, newStore, skip...)` | Runs all contracts; the names in `skip` are reported as skipped |

## Contracts

| Contract                         | Checks                                                                                        |
|----------------------------------|-----------------------------------------------------------------------------------------------|
| `AllowAllTakesEverySlot`         | An admission takes one slot on every key and reports the state after it, in input order       |
| `AllowAllRefusalTakesNothing`    | A refusal by one key takes no slot on the admitting keys (no slot burning)                    |
| `AllowAllReportsFirstRefusal`    | The `*KeyError` names the lowest refusing index; `Remaining` is 0 exactly for refusing keys   |
| `AllowAllRejectsDuplicateKeys`   | A key named twice fails with `ErrDuplicateKey` and takes nothing                              |
| `AllowAllConcurrentAllOrNothing` | Racing calls over overlapping keys, in both key orders, admit exactly the limit, never partly |

## Usage

```go
func TestMultiStorageContract(t *testing.T) {
    t.Parallel()
    storagetest.Run(t, func(testing.TB) storages.MultiStorage { return memory.New() })
}
```
