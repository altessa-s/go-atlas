# factory

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/factory"
```

Package `factory` provides configuration-based creation of TLS configurations and certificate providers.
Reads from `config.TlsClient` and `config.TlsProvider` to select backends (File, Vault, Let's Encrypt).

## Usage

```go
f := factory.New(factory.WithLogger(logger))

// Client TLS config
clientCfg, err := f.CreateClientConfigFromConfig(&cfg.TLS.Client)

// Server certificate providers
providers, err := f.CreateProvidersFromConfig(&cfg.TLS.Provider)
defer providers.Close(ctx)
```
