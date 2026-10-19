# clock

```go
import "github.com/altessa-s/go-atlas/core/time/clock"
```

An injectable source of the current time that can also wait (`Clock`), the system clock (`Real`) and a manually driven fake for tests (`Fake`).
No external dependencies.

| API | Purpose |
|-----|---------|
| `Clock` | `Now() time.Time` and `Sleep(ctx, d) error` (returns `ctx.Err()` as soon as `ctx` ends) |
| `Real()` | `time.Now` and a timer-backed `Sleep` |
| `NewFake(t)` / `Fake` | Reads the set time; `Advance(d)`, `Set(t)`; `Sleep` advances the time by `d` at once and returns `ctx.Err()`; safe for concurrent use |

The method values fit the usual seams: `fake.Now` as a `func() time.Time` (for example `keyset.WithClock`), `fake.Sleep` as a `retry.SleepFunc`
(`retry.WithSleep`). A `Fake` also satisfies the `Now`-only clock interfaces of `auth/jwt` and `auth/selfjwt`.

```go
fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
codec, _ := keyset.New(key, keyset.WithTTL(time.Hour), keyset.WithClock(fake.Now))
token, _ := codec.Issue(payload, bindings)
fake.Advance(2 * time.Hour)
_, err := codec.Resolve(token, bindings) // expired
```
