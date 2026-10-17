# embed

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
```

Package `embed` implements an OPA policy source backed by an embedded filesystem. OPA policies compile directly into the binary, so no external policy
files are needed at runtime.

## Usage

```go
import (
    "embed"

    embedsource "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
)

//go:embed policies/*
var policiesFS embed.FS

source, err := embedsource.New(policiesFS, "policies")
if err != nil {
    return err
}
defer source.Close()

bundle, err := source.Fetch(ctx)
```

## Key Types

| Type | Description |
|------|-------------|
| `Source` | Policy source implementation backed by `fs.FS` |

## Methods

| Method | Description |
|--------|-------------|
| `New` | Creates a new embedded policy source from an `fs.FS` |
| `Fetch` | Retrieves the policy bundle from the embedded filesystem |
| `Name` | Source identifier |
| `Close` | Marks the source closed; later `Fetch` calls return `opa.ErrSourceClosed` (repeated closes succeed) |
| `Dir` / `Extensions` | Configured directory and policy file extensions |

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithExtensions` | `[".rego"]` | File extensions loaded as policies |
| `WithIncludeData` | false | Load `.json` files as OPA data alongside Rego policies |
| `WithChecksums` | nil | SHA-256 hex digests per policy file; a mismatch, unlisted or missing file fails `Fetch` |
| `WithLogger` | discard | Structured logger (`*slog.Logger`) |

## Features

- Zero runtime dependencies on external policy files
- Policy source files are embedded at build time and compiled by the manager at runtime
- Immutable policies prevent tampering
- Compatible with Go's `embed` package for build-time inclusion