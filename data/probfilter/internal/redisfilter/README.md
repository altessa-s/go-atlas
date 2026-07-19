# redisfilter

```go
import "github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
```

Shared plumbing for Redis-backed probabilistic filter storages built on RedisBloom module commands. The Bloom (`BF.*`) and Cuckoo (`CF.*`)
storages differ almost only in the command prefix, the reserve arguments, and the filter label in wrapped errors — `Core` captures the common
execution, error-wrapping, batching, and create-on-first-use behavior, parameterized by a `Commands` set.

## Key types

| Type       | Description                                                                                       |
|------------|---------------------------------------------------------------------------------------------------|
| `Commands` | RedisBloom command verbs of one filter type: `Label`, `Exists`, `Add`, `AddBatch`, `BatchTokens`, `Reserve`, `Info` |
| `Core`     | Executes the commands against one filter key; safe for concurrent use                              |

## Core methods

| Method              | Description                                                                                  |
|---------------------|----------------------------------------------------------------------------------------------|
| `MightExist`        | Membership check via the exists command                                                      |
| `Add`               | Single insert; creates the filter on a "not exist" error and retries once                    |
| `AddBatch`          | Chunked multi-insert (batches of ≤1000 command arguments); same create-and-retry behavior    |
| `EnsureFilter`      | Reserve with the configured arguments; an already-existing filter is not an error            |
| `Reserve`           | Reserve with explicit arguments; any failure is an error (used by Bloom `Reset`)             |
| `DeleteFilter`      | `DEL` of the filter key                                                                      |
| `Info`              | Raw `*.INFO` reply; reports `found=false` (not an error) when the filter does not exist yet  |
| `FilterKey`         | The fully prefixed Redis key                                                                 |

## Helpers

| Function     | Description                                                              |
|--------------|--------------------------------------------------------------------------|
| `InfoFields` | Iterator over the alternating key/value pairs of a `*.INFO` reply        |
| `ToInt64`    | Converts an info reply value (`int64`, `int`, or string) to `int64`      |

## Error wrapping

All failures are wrapped with `core/errors.WrapOperation` using operation strings derived from `Commands.Label`, e.g. for `Label: "Bloom"`:
`check existence in Redis Bloom filter`, `add to Redis Bloom filter`, `batch add to Redis Bloom filter`, `create Redis Bloom filter`,
`delete Redis Bloom filter`, `get Redis Bloom filter info`. These strings are a compatibility contract of the public storages — keep them
byte-identical.

## Usage

See the consumers: [`bloom/storages/redis`](../../bloom/storages/redis/README.md) and [`cuckoo/storages/redis`](../../cuckoo/storages/redis/README.md).
