# Go 1.26 migration plan

## Goal

The repository already targets Go 1.26 (`go 1.26.0` in `go.mod`, `tests/integration/go.mod`, `devtools/go.mod`; `toolchain go1.26.5`; CI `go-version:
"1.26"`; golangci-lint v2.10.1). The goal is to (a) adopt Go 1.26 language/library features where they simplify code, and (b) delete go-atlas code that
duplicates the Go ≤1.26 standard library.

### Acceptance criteria

- No `//go:build !go1.26` (or older-version) files remain.
- `GOTOOLCHAIN=go1.26.5 go fix -diff ./...` produces an empty diff in all three modules.
- Exported duplicates listed in Phase 3 are removed and every call site migrated (no deprecated shims, per AGENTS.md).
- `GOTOOLCHAIN=go1.26.5 go build ./...` (each module), `go vet ./...`, `make lint`, `go test -race -count=1 ./...`, `make check-architecture` pass;
  `make test-integration` passes for touched storages.
- `make bench` shows no regression > 5% on touched hot paths versus a baseline captured before Phase 2.
- AGENTS.md / CLAUDE.md / package READMEs / doc.go no longer reference removed APIs.

## Evidence (collected with Go 1.26.5 toolchain)

- New 1.26 API (from `$GOROOT/api/go1.26.txt`): `errors.AsType`, `slog.NewMultiHandler`, `reflect` iterators (`Type.Fields/Methods/Ins/Outs`,
  `Value.Fields/Methods`), `bytes.Buffer.Peek`, `testing.{T,B,F}.ArtifactDir`, `testing/cryptotest.SetGlobalRandom`, `net.Dialer.Dial{TCP,UDP,IP,Unix}`,
  `net/http.ClientConn`, `netip.Prefix.Compare`, `os.Process.WithHandle`, `crypto/hpke`, `crypto/mlkem` additions; `httputil.ReverseProxy.Director`
  deprecated. Language: `new(expr)`.
- `go fix -diff ./...` (root module) proposes changes in 74 files: ≈60 `&v`/wrapper → `new(expr)`, 12 reflect field iterators, 10 `maps.Collect/Copy`, 7
  `reflect.TypeFor`, 5 `slices.Contains`, 6 `strings.SplitSeq/Cut`, 6 `min/max`, 4 `t.Context()`. It also annotates `ptr.Wrap`,
  `testhelpers.StringPtr/IntPtr/TimePtr` and local `ptrInt` with `//go:fix inline`.

## Phase 0 — Baseline

Capture `make bench` output (root module) to a file outside the repo for later comparison (Green Tea GC is default in 1.26).

## Phase 1 — Remove dead pre-1.26 build-tag branches

- `core/errors/as_type.go` (`//go:build !go1.26`) and `as_type_go126.go`: delete `coreerrs.AsType` entirely; migrate 35 callers (`coreerrs.AsType` 32,
  `coreerrors.AsType` 3) to `errors.AsType` (stdlib already used in 14 places).
- `observability/slog/handler/multi/handler.go` (`//go:build !go1.26`): delete the dead branch only; rename `handler_go126.go` → `handler.go` and drop
  the build tag. `multi.Handler` (both `NewHandler` and `NewConcurrentHandler`) is **kept**: it is a lifecycle adapter, not a duplicate — its
  `Handlers()` method is what `observability/slog.Shutdown` (`helpers.go`, `shutdownHandler` via `InnerHandlers`) uses to reach and drain child
  handlers, and `slog.MultiHandler` exposes no children. Callers are not migrated to `slog.NewMultiHandler`. Add a regression test: a logger built with
  `multi.NewHandler` over a buffered child (also via `With`/`WithGroup` derived loggers) is fully drained by `slogx.Shutdown`.

## Phase 2 — Mechanical `go fix` (separate commit per module, no hand edits)

Run `go fix ./...` twice per module: pass 1 rewrites idioms and adds `//go:fix inline` to trivial wrappers; pass 2 inlines the annotated wrappers at
call sites. Review the diff, then `gofmt`/`gci` via `make fmt`.

## Phase 3 — Delete go-atlas helpers duplicating the stdlib

| go-atlas API | Replacement | Callers | Notes |
|---|---|---|---|
| `core/types/ptr.Wrap` | `new(v)` | 26 | keep `WrapNonZero`, `Unwrap` (no stdlib equivalent) |
| `internal/testhelpers.StringPtr/IntPtr/TimePtr` | `new(v)` | 13 | update testhelpers doc.go/README, AGENTS.md list |
| `core/collections/slices.Values`, `Backward` | `slices.Values`, `slices.Backward` | 25 | identical semantics |
| `core/collections/slices.Chunk` | `slices.Chunk` | 4 | stdlib panics on size<1 and clips chunk capacity; guard size at callers |
| `core/collections/slices.Any` | `slices.ContainsFunc` | ≈3 | already a one-line wrapper; `All` stays |
| `core/collections/maps.Keys`, `Values` | `maps.Keys`, `maps.Values` | 6 | identical semantics |
| `core/runtime.AddCleanup` | `runtime.AddCleanup` | 7 | one-line wrapper; drop `Cleanup` interface |
| `core/runtime.ClearFinalizer` | `runtime.SetFinalizer(obj, nil)` | 3 | one-line wrapper |
| `core/time.TimerStopAndDrain` | `t.Stop()` | 21 | with `go ≥ 1.23` in go.mod timer channels are synchronous; no stale value after Stop. Also audit manual `select { case <-t.C: default: }` drains |

Not duplicates (kept): `core/text/strings.ToPtr` (nil for blank strings), `coremaps.Merge` (non-mutating), `coremaps.WeakMap` (already built on `weak`),
`core/text/strings.SplitSeq` (options), `ImmutableMap`.

Package hygiene per AGENTS.md: if a package becomes empty, delete it with doc.go/README; otherwise update doc.go, README, tests and benchmarks of each
touched package.

## Phase 4 — Targeted adoption of 1.26 APIs

- Convert the remaining `errors.As` (20 prod, 6 test) to `errors.AsType` where the target is branch-local.
- 52 `NumField()` loops not rewritten by `go fix`: convert to `Type.Fields()/Value.Fields()` only with before/after benchmarks in hot paths
  (`domain/behavior`, `domain/converter`, `domain/normalizer`, `config/loader`).
- Not adopted: `testing/cryptotest.SetGlobalRandom` — it requires `*testing.T` and rejects parallel tests, while the shared
  `testhelpers.GenerateRSAKey/SelfSignedCert` take `testing.TB` and serve parallel tests and benchmarks.
- Not adopted (no use case): `net/http.ClientConn`, `Dialer.DialTCP`, `crypto/hpke`, `mlkem`, `bytes.Buffer.Peek`.

## Phase 5 — Remaining legacy idioms

`sort.Strings` → `slices.Sort` (`data/probfilter/factory/builder.go:391`); `math/rand` v1 → `math/rand/v2`
(`transport/grpc/interceptors/metrics/metrics.go`, seeded stream → `rand.New(rand.NewPCG(...))`, deterministic output must be preserved or tests
updated); `wg.Add(1)`+`go` → `wg.Go` (2 prod, 2 test); `os.MkdirTemp` → `t.TempDir()` (4 tests). `context.Background()` in tests (151, many legitimate
inside `t.Cleanup`) stays on-touch.

## Phase 6 — Documentation

AGENTS.md `core/*` table and testhelpers list, CLAUDE.md modern-idioms list (add `new(expr)`, `errors.AsType`), READMEs/doc.go of touched packages,
CHANGELOG entry listing removed exported APIs with replacements.

## Phase 7 — Guardrails

Enable the `modernize` linter in `.golangci.yml` (only `copyloopvar` is enabled today). Add a CI step that fails when `go fix -diff ./...` is non-empty.

## Decision: breaking changes

Phase 3 removes exported APIs (library consumers break). Decided: delete immediately, matching the AGENTS.md "no shims" rule, and document replacements
in CHANGELOG.

## Verification

After each commit (the Makefile has no library `build` target): `GOTOOLCHAIN=go1.26.5 go build ./...` in the root, `tests/integration` and `devtools`
modules; `go vet ./...`; `make lint`; `go test -race -count=1 ./...`; `make check-architecture`. After Phases 3–4: `make bench` vs Phase 0 baseline;
`make integration-up`, `make test-integration`, `make integration-down` for touched storages. Final: `GOTOOLCHAIN=go1.26.5 go fix -diff ./...` is empty
in every module; `make security-scan` passes.

## Risks

- `slices.Chunk` panic on size<1 vs current empty-iterator behavior.
- Downstream consumers setting `GODEBUG=asynctimerchan=1` would lose drain semantics after removing `TimerStopAndDrain`.
- reflect iterators may be slower than indexed loops on hot paths (benchmark-gated).
- `math/rand/v2` changes deterministic sequences used by tests.
- `go fix` under a newer local toolchain (1.27) could apply 1.27-only modernizers; always pin `GOTOOLCHAIN=go1.26.5`.

## Implementation notes

Deviations and findings recorded while executing the plan:

- `go fix` (Go 1.26.5) `mapsloop` rewrote six test declarations `x := make(map…)` + loop into `x = maps.Collect(…)`, dropping the declaration; they
  were corrected to `:=` by hand.
- The 19 remaining `NumField()` loops (`domain/converter`, `config/loader`, `domain/behavior`, `domain/normalizer`, `data/mongo`, `security/secrets`,
  …) use the index for more than the field itself — parallel `dst.Field(i)`, cached per-index metadata, index paths — so `Type.Fields()` /
  `Value.Fields()` would not simplify them. They are left as indexed loops.
- `errors.As` stays where the target is an interface that does not embed `error` (`transport/http/server/writer`: `Coder`, `Messager`,
  `HTTPStatuser`): `errors.AsType[E error]` cannot express it.
- `transport/grpc/client/health.go` keeps `wg.Add(1)` + `go`: wrapping the call in `wg.Go(func(){…})` moves the `contextcheck` finding to the whole
  `attach` function. `transport/grpc/client/pool/state_tracker.go` uses `wg.Go` before releasing the lock, preserving the register-under-lock order.
- `os.MkdirTemp` remains in `Example` functions (no `testing.T`) and in the Linux landlock subprocess test: that code runs in a child process
  without a `testing.T`, and once Landlock is applied the process may not remove the directory anyway.
- The `modernize` linter in golangci-lint v2.10.1 ships `slicesbackward`, which `go fix` in 1.26.5 lacks; the nine flagged reverse loops were
  rewritten with `slices.Backward`.
- `stditerators` rewrote three value-only loops (`domain/converter.isStructZero`, `isSparseStructEmpty`, `domain/behavior.cloneValue`) to
  `Value.Fields()`, which also builds a `StructField` per field: `SparseMerge` went from 0 to 4 allocs/op and `Clean` from 7 to 22. They are indexed
  again with `//nolint:modernize`, and the CI `go fix -diff` step runs with `-stditerators=false`.
- The CI `go fix -diff` step covers the root and `tests/integration` modules; `devtools` holds only a build-tagged tools file, and `go fix` exits
  non-zero on a module with no packages.
- Benchmarks: an interleaved A/B run of the touched hot paths against the pre-migration commit (same machine, same conditions) shows no change
  beyond noise — largest delta `UnwrapEnvValue_Plain` +2.7%, allocations equal or lower. A sequential before/after run was unusable: an untouched
  `StdMap_Get` control moved +58% between the two runs because of machine load.
