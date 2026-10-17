# spoolbudget

```go
import "github.com/altessa-s/go-atlas/core/io/spoolbudget"
```

Bounds the aggregate on-disk bytes held by concurrent [`spool`](../spool/README.md) materializations. A single process-wide `Budget` reserves each
spool's per-call cap before writing and blocks (back-pressure) until it fits, so the sum of reserved bytes never exceeds the limit — a peak bound on
reservations, not a post-hoc average. Actual disk usage can briefly exceed it: `Close` releases the reservation before the file is removed, and oversize
detection writes one extra byte before rejecting.

## Key types

| Symbol | Description |
|--------|-------------|
| `Budget` | Shared byte ceiling charged across every call site; safe for concurrent use |
| `BudgetedSpool` | A `spool.Spool` whose size is held against the `Budget` until `Close` |
| `New(limitBytes int64) *Budget` | Builds a budget; a non-positive limit returns `nil` — a disabled, passthrough budget |
| `Budget.Spool(ctx, r, maxBytes, opts ...spool.Option)` | Reserves `maxBytes`, materializes `r` via `spool.New`, and returns a `*BudgetedSpool`; `ctx` governs the wait |
| `Budget.Limit() int64` | Configured byte ceiling, or `0` when the budget is disabled |

## Behavior

| Receiver / args | Effect |
|-----------------|--------|
| `nil` budget or non-positive limit | Disabled: `Spool` just materializes `r` with no reservation |
| non-positive `maxBytes` | Per-spool unbounded: no reservation, no `spool.WithMaxBytes` cap |
| enabled budget, `maxBytes > 0` | Reserves `maxBytes` up front, releases the over-reservation once the real size is known, holds the actual size until `Close` |

A `nil` `*Budget` is a valid disabled budget — callers invoke its methods without a nil check.

## Usage

```go
// One budget per process, shared by every worker.
budget := spoolbudget.New(512 << 20) // 512 MiB ceiling across all in-flight spools

// Per request: reserve the cap, materialize, re-read, release on Close.
sp, err := budget.Spool(ctx, body, 32<<20) // 32 MiB per-spool cap
if err != nil {
    return err // includes a wait canceled via ctx, or spool.ErrTooLarge
}
defer sp.Close() // releases the held bytes back to the budget

data, err := io.ReadAll(sp)
```

When the budget is enabled, `maxBytes` must be positive and not exceed the limit so the reservation can always be satisfied; a `maxBytes` larger than
the limit could only unblock when `ctx` is canceled.
