# Contributing to go-atlas

Thank you for your interest in contributing to go-atlas! This guide will help you get started.

## Prerequisites

- **Go 1.25+** — [download](https://go.dev/dl/)
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

**Types:** `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`, `perf`

**Scope** is the top-level package (e.g., `transport`, `config`, `data`).

Examples:

```
feat(transport): add gRPC reflection support
fix(data/cache): handle nil pointer in Redis fallback
docs(README): add quick start examples
test(domain/converter): add benchmark for slice conversion
```

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
| `make lint` | Run golangci-lint (34 linters) |
| `make test` | Run all tests |
| `make test-verbose` | Run tests with verbose output |
| `make test-race` | Run tests with race detector |
| `make test-shuffle` | Run tests with randomized order |
| `make test-coverage` | Run tests with coverage report |
| `make test-all` | Run tests with race, shuffle, and coverage |
| `make test-coverage-func` | Per-package coverage breakdown |
| `make bench` | Run all benchmarks |
| `make bench-count` | Run benchmarks for benchstat |
| `make dupl` | Find code duplication |
| `make dupl-check` | Fail on code duplication |
| `make copyright` | Add copyright headers |
| `make generate` | Run go generate |
| `make tidy` | Run go mod tidy |
| `make security-scan` | Run govulncheck and gosec |

## Pull Request Process

1. Ensure `make lint` and `make test-all` pass.
2. Update `CHANGELOG.md` under the `[Unreleased]` section if your change is user-facing.
3. Fill out the PR template completely.
4. One approving review is required before merging.

## Reporting Issues

- **Bugs**: Use the [Bug Report](https://github.com/altessa-s/go-atlas/issues/new?template=bug_report.yml) template.
- **Features**: Use the [Feature Request](https://github.com/altessa-s/go-atlas/issues/new?template=feature_request.yml) template.
- **Security**: See [SECURITY.md](SECURITY.md) for vulnerability reporting.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
