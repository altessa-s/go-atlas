# factory

```go
import s3factory "github.com/altessa-s/go-atlas/factory/ext/tlsutils/s3"
```

Maps the `s3` section of the TLS provider configuration to an
[S3 certificate provider](https://github.com/altessa-s/go-tlsutils/tree/develop/providers/s3) for
[`factory/ext/tlsutils`](..), which builds only the file and Let's Encrypt providers.

## Usage

```go
providers, err := tlsfactory.New(cfg.TLSProvider).
    UseProvider(tlsproviders.ProviderTypeS3, s3factory.Builder(nil)).
    Build(ctx)
```

| Function          | Description                                                                                                         |
|-------------------|---------------------------------------------------------------------------------------------------------------------|
| `Builder(client)` | Provider builder for the `s3` type; a `nil` client is built from the section (endpoint, region, static credentials) |
