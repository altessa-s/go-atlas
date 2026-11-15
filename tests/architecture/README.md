# architecture

Repository tests that enforce the production dependency boundaries documented in [Architecture](../../docs/architecture.md). This directory belongs
to the root Go module and runs with `go test ./...`; [integration](../integration/README.md) is a separate module.

## Usage

```sh
make check-architecture
# or
go test ./tests/architecture
```

## Coverage

| Check | Scope |
|-------|-------|
| Production imports | Parses all platform files, including files excluded by the current build tags |
| Dependency boundaries | Core, configuration schemas, factories, domain, backend-neutral listing, transport implementation imports and consumer-side seams |
| Concrete-type seams | AST check rejects listed implementation selectors in production consumers, including aliased imports and build-tagged files |
| Driver-free contracts | Full `go list` dependency set of the backend-neutral contracts on `linux`, `darwin` and `windows` |
| Rule regressions | Allowed and forbidden dependency examples, including aliased concrete-type violations and interface-typed fields |

Test files, hidden directories, vendored code, development tools and the `tests` tree are excluded from the production import scan. CI runs the
same Makefile target. These tests have no production API or hot path, so no implementation or benchmark file is needed.

SQL field schemas may depend only on the shared filter contract, core helpers, and the standard library. MariaDB translation cannot import
PostgreSQL semantics. Regression cases cover both forbidden edges and the SQL listing engine's neutral schema dependency.
