# gcp

```go
import "github.com/altessa-s/go-atlas/security/secrets/providers/gcp"
```

Package `gcp` provides a Google Cloud Secret Manager provider for the Atlas secrets system. Supports CRUD
operations, concurrent retrieval, base64 encoding, and label-based filtering.

## Usage

```go
store, err := gcp.New[MySecret](projectID, saKeyPath,
    gcp.WithKeyDecoder(base64Dec),
    gcp.WithValueDecoder(jsonDec),
    gcp.WithLabels(map[string]string{"env": "prod"}),
)
```
