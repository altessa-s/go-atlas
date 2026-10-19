# memory

```go
import "github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
```

Package `memory` provides an in-process `generation.Store`. A bump is visible to every later `Current` call of the same process at once and to no
other process. It never fails. Use it for a single replica, in tests, or through `cache.NewGenerations`, which defaults to it.

```go
store := memory.New()
gen, _ := store.Current(ctx, "catalog")
next, _ := store.Bump(ctx, "catalog") // next > gen
```

A namespace is seeded with `generation.Seed` of the clock when first used, and `Bump` returns the larger of the previous generation plus one and
the seed of the current time. Generations therefore keep growing across a process restart, which matters when the cache storage outlives the
process (Redis).

| Option | Default | Description |
|--------|---------|-------------|
| `WithClock` | `clock.Real()` | Clock for seeding |
