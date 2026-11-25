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
| Test fixtures | Runtime imports of `testing`, `internal/testhelpers` and packages ending in `test` are rejected. Fixture packages use the `*test` suffix or live under `internal/testhelpers`. |
| Module dependencies | Each declared edge needs a direct non-test import between its modeled modules. Consumers may still use transitively reachable modules. |
| MongoDB adapters | Only `store/mongo` and its subpackages may import `go-mongox`. Production imports of go-aip remain limited to its root module. |
| Dependency boundaries | Core, configuration schemas, factories, domain, backend-neutral listing, transport implementation imports and consumer-side seams |
| Concrete-type seams | AST check rejects listed implementation selectors in production consumers, including aliased imports and build-tagged files |
| Driver-free contracts | Full `go list` dependency set of the backend-neutral contracts on `linux`, `darwin` and `windows` |
| Planned gRPC module split | Loads current gRPC package edges in a temporary Go module layout with the planned path rewrites |
| Public Config structs | Rejects exported `*Config` structs with exported fields outside `config/` and `proto/` (sole exception: `coord/leadelect/providers.Config`) |
| Optgen field tags | Validates `optgen:"manual"` and `opt:"-"`, forbids `optgen:"-"`, and requires doc comments on non-generated fields |
| Factory mirror layout | Enforces `factory/<path>` mirrors `<path>` and external factories live under `factory/ext/<module>` |
| Factory health dependencies | Rejects references to `health.Coordinator` in `factory/` outside `factory/observability/health` |
| Auth option roles | Verifies variadic constructor option types are mutually distinct per role in `auth/jwt`, `auth/selfjwt`, and `auth/oauth2client` |
| Public transport APIs | Checks public transport declarations for internal types and runs both server lifecycles from a separate consumer module. |
| Transport error logging | Typed AST check on `./transport/...` and `./internal/transport/...` rejects raw `error` arguments and `.Error()` strings outside `internal/diagnostics.LogError` and error classifiers. |
| Rule regressions | Allowed and forbidden dependency examples, including aliased concrete-type violations and interface-typed fields |

Test files, hidden directories, vendored code, development tools and the `tests` tree are excluded from the production import scan. The module check
maps split-out libraries and relocated packages to their planned module roots. Only sources in this repository supply evidence for an outgoing edge. CI
runs the same Makefile target. These tests have no production API or hot path, so no implementation or benchmark file is needed.

The public transport API check (`TestTransportPublicAPI`) inspects all non-internal packages under `transport/...` and rejects any `/internal/` type
in an exported declaration with no allowlist. `TestExternalServerAPI` compiles and runs both server lifecycles from an isolated external Go module
(`testdata/serverapi`).
