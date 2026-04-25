# storages

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
```

Package `storages` defines the `Storage` interface for Cuckoo filter backends. Implementations live in subpackages and are injected into the
`cuckoo.Filter` to supply the underlying bucket array and fingerprint operations.

## Key types

| Type / Interface | Description                                                         |
|------------------|---------------------------------------------------------------------|
| `Storage`        | Interface: MightExist, Add, AddBatch, Delete, Close                 |
| `StatsProvider`  | Optional interface for backends that expose filter statistics        |

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory Cuckoo storage           |
| [redis](./redis)       | Redis-backed Cuckoo storage        |
