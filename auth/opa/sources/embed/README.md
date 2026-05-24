# embed

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
```

Package `embed` implements an OPA policy source backed by an embedded filesystem.
It allows OPA policies to be compiled directly into the binary, eliminating the need for external policy files at runtime.

## Usage

```go
//go:embed policies/*
var policiesFS embed.FS

source, err := embed.New(policiesFS, "policies")
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
| `Close` | No-op for embedded sources (satisfies interface) |

## Features

- Zero runtime dependencies on external policy files
- Fast startup with pre-compiled policies
- Immutable policies prevent tampering
- Compatible with Go's `embed` package for build-time inclusion