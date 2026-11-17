# dlockit

```go
import "github.com/altessa-s/go-atlas/tests/integration/dlockit"
```

Live NATS, MongoDB, PostgreSQL, MariaDB, and MySQL lock tests verify mutual exclusion, fencing, renewal, expiry, and cleanup. `Critical` records visits
and detects overlapping holders.

## Key types

| Type       | Purpose                                                    |
|------------|------------------------------------------------------------|
| `Critical` | Records critical sections and reports overlap violations   |

## Lease timing

| Cases                                               | TTL        | Reason                                                        |
|-----------------------------------------------------|------------|---------------------------------------------------------------|
| MongoDB and SQL held-lock contracts and contention   | 15 seconds | Allows slow acquisitions under load                            |
| MongoDB release, cleanup index; SQL cleanup          | 15 seconds | Does not depend on natural expiry                              |
| MongoDB and SQL `ExpiredLeaseIsTakenOver`             | 15 seconds | The contract explicitly expires the stored lease               |
| MongoDB and SQL `Renewal`, `ExpiredLeaseStaysLost`     | 1 second   | Exercises renewal timing without extending suite runtime       |
| Hard leases; SQL renewal blocked on a row            | 1 second   | Exercises actual expiry                                        |
| NATS scenarios                                      | 3 seconds  | Preserves the existing renewal and abandoned-key expiry timing |

MongoDB collections and their indexes are initialized before acquisition timing. Uncontended contract acquisitions and the initial MongoDB hard
lease acquisition retry expired acquisition replies; assertions that a held lock rejects contenders remain single attempts.

## Usage

Start the [integration stack](../README.md) with `MONGO_PORT=27029`, then run:

```sh
MONGO_PORT=27029 MONGO_URI="mongodb://127.0.0.1:27029/?directConnection=true" go test -C tests/integration -race -count=3 -p 1 -v ./dlockit/...
```

The SQL retention scenarios use `PurgeExpired(ctx) (int64, error)` to drain expired lock rows across bounded batches; cleanup grace preserves
active leases and controls when fencing history can reset.
