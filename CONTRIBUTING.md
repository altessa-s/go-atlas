# Contributing to go-atlas

Thank you for your interest in contributing to go-atlas! This guide will help you get started.

## Prerequisites

- **Go 1.26+** — [download](https://go.dev/dl/)
- **Make** — for running project targets
- **Git** — for version control

## Getting Started

1. **Fork** the repository on GitHub.

2. **Clone** your fork:

   ```bash
   git clone https://github.com/<your-username>/go-atlas.git
   cd go-atlas
   ```

3. **Create a branch** from `develop`:

   ```bash
   git checkout -b feat/my-feature develop
   ```

4. **Make your changes**, then run checks:

   ```bash
   make fmt
   make lint
   make test-all
   ```

5. **Commit** using [Conventional Commits](#commit-messages) and push:

   ```bash
   git push origin feat/my-feature
   ```

6. **Open a Pull Request** against the `develop` branch.

## Branch Naming

Use the following prefixes:

| Prefix | Purpose |
|--------|---------|
| `feat/` | New features |
| `fix/` | Bug fixes |
| `docs/` | Documentation changes |
| `refactor/` | Code refactoring |

## Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:** `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`, `perf`, `build`, `style`

**Scope** must match one of the allowed scopes in [`commit_scopes.txt`](commit_scopes.txt). The scope represents the package or module affected by the change.

Examples:

```
feat(transport/grpc): add gRPC reflection support
fix(data/cache): handle nil pointer in Redis fallback
docs(README): add quick start examples
test(domain/converter): add benchmark for slice conversion
build(root): update Go version to 1.25
```

### Automated Validation

- **GitHub Actions** automatically validates all commit messages and PR titles
- **Local validation** can be enabled with: `git config core.hooksPath .githooks`
- Run `.github/scripts/validate-commits.sh` to check your commits before pushing

## Code Style

### Formatting

- Run `make fmt` before committing — this runs `gci` with the project's import ordering.
- Maximum line length: **156 characters** (enforced by `lll` linter).

### Import Ordering

Imports must follow this order (enforced by `gci` via `.golangci.yml`):

1. Standard library
2. Third-party packages (default)
3. `google.golang.org`
4. `golang.org`
5. `github.com/altessa-s/go-atlas`
6. `github.com/altessa-s` (other altessa packages)
7. Blank imports
8. Aliases

### Copyright Headers

All `.go` files must include the copyright header. Run `make copyright` to add it automatically.

```go
// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make fmt` | Format code and sort imports |
| `make lint` | Run golangci-lint with the repo config (`.golangci.yml`) |
| `make test` | Run all tests |
| `make test-verbose` | Run tests with verbose output |
| `make test-race` | Run tests with race detector |
| `make test-shuffle` | Run tests with randomized order |
| `make test-coverage` | Run tests with coverage report |
| `make test-all` | Run tests with race, shuffle, and coverage |
| `make test-coverage-func` | Per-package coverage breakdown |
| `make bench` | Run all benchmarks |
| `make bench-count` | Run benchmarks for benchstat |
| `make bench-gate` | Fail if hot-path benchmarks regressed against `benchmarks/baseline.txt` |
| `make bench-baseline` | Regenerate `benchmarks/baseline.txt` on this machine |
| `make fuzz-list` | List every fuzz target |
| `make fuzz-seed` | Run fuzz targets over their seed corpus only (no mutation) |
| `make fuzz` | Fuzz every target for `FUZZTIME` each (`FUZZTIME=30s FUZZPKG=./...`) |
| `make dupl` | Find code duplication |
| `make dupl-check` | Fail on code duplication |
| `make copyright` | Add copyright headers |
| `make generate` | Run go generate |
| `make tidy` | Run go mod tidy |
| `make security-scan` | Run govulncheck and gosec |

## Fuzzing

`go test ./...` already executes every fuzz target over its `f.Add` seeds, so seeds act as ordinary regression cases and a PR needs nothing extra.
Mutation is separate: `make fuzz` walks each target for `FUZZTIME` (`go test -fuzz` accepts one target per package per invocation), and the `Fuzz`
workflow does the same nightly with a larger budget, restoring the generated corpus between runs so coverage compounds.

Write a fuzz target when the input is attacker-controlled or there is a property worth stating — a round trip, an allowlist that must hold, an
invariant the code documents. "Does not panic" is the floor, not the goal: a target with no assertion passes for the wrong reasons. When a run
fails, Go writes the minimized reproducer to the package's `testdata/fuzz/<Target>/`; commit it, so the case is pinned forever in the seed corpus.

## Benchmark regression gate

`benchmarks/hotpaths.txt` lists the hot-path benchmarks a change must not slow down; `benchmarks/bench-gate.sh` runs them (`-count=6`,
`-benchtime=200ms`) and compares the results with `benchstat`, pinned in the `devtools` module. A change fails the gate when:

| Metric | Fails when | Why |
|--------|------------|-----|
| `sec/op` | slower by more than 15%, significant at p < 0.05, in two measurements | Time is noisy: a margin, significance and a rerun are needed |
| `allocs/op` | the median grows by any amount | Allocation counts are deterministic for a code path, so they are compared strictly |
| missing | a benchmark has no result in the new run | A hot path must not drop out of the gate silently |

When `sec/op` is the only failure, the gate measures everything once more and fails only on benchmarks that regress again; one noisy run on a busy
machine is not enough. `B/op` is reported but not gated: amortized buffer growth makes it fractional and noisy, and a new allocation already shows
up in `allocs/op`. Thresholds, count and benchtime can be overridden through the `BENCH_GATE_*` variables documented at the top of the script.

Timings only compare on the same machine, so the committed `benchmarks/baseline.txt` belongs to the machine that produced it and the gate
refuses a baseline whose `goos`/`goarch`/`cpu` differ from the current run. Locally, run `make bench-baseline` on the base commit (or keep
the committed one if it is yours), then `make bench-gate` on your branch. In CI the `Bench` workflow ignores the committed baseline: it checks
out the PR and its base side by side and benchmarks both on the same runner, interleaving them round by round so runner drift hits both
equally. When you add, rename or remove an entry in `hotpaths.txt`, regenerate the baseline in the same change.

## Pull Request Process

1. Ensure `make lint` and `make test-all` pass.
2. Write conventional commit messages: release notes are generated from them by goreleaser at release time, there is no
   manually maintained `CHANGELOG.md`.
3. Fill out the PR template completely.
4. One approving review is required before merging.

## Reporting Issues

- **Bugs**: Use the [Bug Report](https://github.com/altessa-s/go-atlas/issues/new?template=bug_report.yml) template.
- **Features**: Use the [Feature Request](https://github.com/altessa-s/go-atlas/issues/new?template=feature_request.yml) template.
- **Security**: See [SECURITY.md](SECURITY.md) for vulnerability reporting.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
