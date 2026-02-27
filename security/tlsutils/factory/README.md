# factory

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/factory"
```

Package `factory` provides configuration-based creation of TLS configurations and certificate providers. Reads from `config.TlsClient` and
`config.TlsProvider` to select backends (File, Vault, Let's Encrypt).
