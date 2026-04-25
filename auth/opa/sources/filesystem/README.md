# filesystem

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
```

Package `filesystem` provides a `PolicySource` implementation that reads OPA policies from the local filesystem with optional hot-reload
support via fsnotify. Reads `.rego` files from a directory (or a single file), computes content-based revisions, and optionally loads `.json`
data files into the OPA data store. Supports SHA-256 checksum verification for policy integrity.

## Options

| Option              | Default      | Description                                                       |
|---------------------|--------------|-------------------------------------------------------------------|
| `WithExtensions`    | `[".rego"]`  | File extensions to include when scanning for policy files          |
| `WithIncludeData`   | false        | Load `.json` files as OPA data alongside Rego policies            |
| `WithLogger`        | discard      | Structured logger (`*slog.Logger`)                                |
| `WithChecksums`     | nil          | SHA-256 hex digests map for policy file integrity verification    |
