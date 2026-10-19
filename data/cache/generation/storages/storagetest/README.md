# storagetest

```go
import "github.com/altessa-s/go-atlas/data/cache/generation/storages/storagetest"
```

Backend-neutral contract suite for `generation.Store` implementations. Every bundled backend runs it, and a custom backend should too:
generational caches rest on seeding from a clock, bumps visible at once to the bumper and within the documented bound to every replica, and
generations that never go backwards.

## Contracts

| Function                 | Verifies                                                                                                   |
|--------------------------|------------------------------------------------------------------------------------------------------------|
| `Run`                    | `Run(t, newBackend, staleness)`: every contract below as parallel subtests, each on a fresh backend        |
| `SeedFromClock`          | A fresh namespace starts at `generation.Seed` of the seed clock, keeps it, and every replica sees the seed |
| `BumpVisibleAtOnce`      | A bump exceeds the previous generation and is returned by the bumper's next `Current`                      |
| `BumpVisibleWithinBound` | Another replica that read the namespace before sees the bump once the staleness bound has passed           |
| `Monotonic`              | Interleaved `Current` and `Bump` never go backwards, also when the seed clock steps back                   |
| `ConcurrentBumps`        | Bumps racing on several replicas all succeed with distinct generations; replicas converge on the highest   |
| `NamespaceIsolation`     | A bump of one namespace leaves the others unchanged on every replica                                       |

## Harness

| Argument or field     | Description                                                                                                |
|-----------------------|------------------------------------------------------------------------------------------------------------|
| `newBackend`          | A `NewBackend`: returns a fresh, isolated `Backend` with the requested number of replicas                  |
| `staleness`           | Visibility bound for other replicas; zero means at once. The suite advances strictly past it               |
| `Backend.Stores`      | The replicas sharing the backend; an in-process backend repeats one store                                  |
| `Backend.SetSeedTime` | Sets the clock fresh namespaces are seeded from; nil when uncontrollable (a live server), seed checks skip |
| `Backend.Advance`     | Advances the clock measuring local staleness; required when `Staleness` is positive                        |

The Redis store seeds from the server clock (miniredis `SetTime`) and measures staleness with its injected clock, so the two are separate.

## Usage

```go
func TestStore_Contract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newBackend, time.Second) // newBackend(tb, replicas) storagetest.Backend
}
```

See [storages/memory](../memory) and [storages/redis](../redis) for the bundled harnesses.
