# tests/integration

Integration suite for `go-atlas`, kept in its own Go module.

```
tests/integration/
├── go.mod                 # separate module — see below
├── docker-compose.yml     # the six backends the suite runs against
└── filterit/              # data/filter: shared corpus + one adapter per backend
```

## Why a separate module

Exercising the SQL filter translators needs `clickhouse-go`, `pgx` and `go-sql-driver/mysql`. Those are test dependencies of a *library*, so
putting them in the root `go.mod` would push all three into the dependency graph and `go.sum` of every project that imports go-atlas. A separate
module with `replace github.com/altessa-s/go-atlas => ../..` keeps the toolkit's own graph clean, the same way `devtools/` does.

The cost: these tests are not part of the root module's `go test ./...`. Run them explicitly.

## Running

```bash
docker compose -f tests/integration/docker-compose.yml up -d --wait
make test-integration
docker compose -f tests/integration/docker-compose.yml down -v
```

Ports are deliberately non-default so the stack never collides with a database already running locally. Every test **skips** rather than fails
when its server is unreachable, so `make test-integration` is green on a machine with no Docker — check the skip count if you expect coverage.

| Backend       | Default address           | Environment override                       |
|---------------|---------------------------|--------------------------------------------|
| ClickHouse    | `127.0.0.1:19001`         | `CLICKHOUSE_ADDR`, `CLICKHOUSE_DB`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD` |
| MariaDB       | `127.0.0.1:13306`         | `MARIADB_DSN`                              |
| PostgreSQL    | `127.0.0.1:15432`         | `POSTGRES_DSN`                             |
| MongoDB       | `127.0.0.1:27019`         | `MONGO_URI`                                |
| Meilisearch   | `http://127.0.0.1:17700`  | `MEILI_URL`, `MEILI_KEY`                   |
| RediSearch    | `127.0.0.1:16379`         | `REDIS_ADDR`                               |

Each test creates its own throwaway table, database or index, named with a timestamp suffix, and drops it on cleanup — two runs against the same
server never collide.

## filterit — the data/filter corpus

One dataset of seven rows and one list of filter expressions run through all six translators. `corpus.go` is pure data; each
`*_integration_test.go` is the adapter that loads the dataset into its backend and executes the translated query.

Two matrices:

| Matrix         | Asserts                                                                                     |
|----------------|---------------------------------------------------------------------------------------------|
| `Cases()`      | The expression translates, the **server accepts it**, and it selects exactly the right rows  |
| `Rejections()` | The expression is refused with a named sentinel — `ErrFieldNotAllowed`, `ErrMaxDepthExceeded`, … |

The middle assertion is the one unit tests cannot make. A clause can match a golden string byte for byte and still be a syntax error, name a
function the server does not have, or type-mismatch on the wire.

### Divergence is asserted, not tolerated

Where a backend returns a *different but correct* answer, `Case.Differs` pins it:

| Expression           | Most backends | Diverges                                                                   |
|----------------------|---------------|----------------------------------------------------------------------------|
| `name == "Alice"`    | `{1}`         | MariaDB, Meilisearch, RediSearch → `{1,5}` (case folding)                  |
| `name > "M"`         | `{5,7}`       | MariaDB, PostgreSQL → `{7}` (collation orders `alice` below `M`)           |
| `name.contains("Al")`| `{1}`         | MariaDB, Meilisearch, RediSearch → `{1,5}`                                 |

Where the case does not apply at all, `Case.Skip` records the reason. Run with `-v` to read them:

```bash
go test -C tests/integration -v ./filterit/ 2>&1 | grep -A1 SKIP
```

### Translator bugs this found

Three bugs surfaced on the first full run. None was caught by the unit tests, which assert on generated strings and never assemble a whole
query against real data. All three are fixed; the cases that exposed them are now ordinary corpus entries.

| Bug                                                                                              | Backends                         | Fix                                                        |
|--------------------------------------------------------------------------------------------------|----------------------------------|------------------------------------------------------------|
| A bare boolean identifier (`active`) was not rendered as a boolean test                           | MongoDB, Meilisearch, RediSearch | `acceptPredicate` at the root and on both logical operands  |
| `in` ignored the field schema and always emitted TAG syntax, so `role in [2,3]` matched nothing   | RediSearch                       | `buildIn` dispatches on `FieldType`                        |
| `field != null` matched every document, including those without the attribute                     | Meilisearch                      | paired with the matching `EXISTS` / `NOT EXISTS` check      |

The first was an asymmetry rather than a design choice: all three translators already special-cased `!active`, and only the positive form was
missing. MongoDB failed loudly with `ErrInvalidExpression`; Meilisearch emitted a bare attribute name the server rejected as a missing operator;
RediSearch emitted it as a free-text term, so the query ran and matched whatever the TEXT fields happened to contain.

The third was the one worth finding. `deletedAt != null` is the shape of a soft-delete filter, and against Meilisearch it silently returned the
deleted documents. `TestMeili_NullFilterSemantics` is its regression guard and still asserts what the bare `IS NOT NULL` would have returned, so
the reason the paired form exists stays visible.

## Adding a backend

Implement the four-method `backend` interface in `harness_test.go` — `name`, `setup`, `search`, `translate` — and call `runCorpus` and
`runRejections` from a `Test<Name>` pair. `setup` must `Skipf` when the server is unreachable rather than failing.
