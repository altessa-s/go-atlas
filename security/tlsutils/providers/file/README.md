# file

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
```

Package `file` provides a file-based TLS certificate provider with optional automatic reloading.
Supports password-protected keys and OCSP stapling.

## Usage

```go
p, err := file.NewWithCertAndKey(certPath, keyPath, password,
    file.WithAutoReload(true),
    file.WithOCSPStapler(stapler),
)
defer p.Close(ctx)

tlsCfg := p.TLSConfig()
```
