# s3

```go
import tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
```

Package `s3` provides a TLS certificate provider that fetches certificates from S3-compatible storage.
It supports automatic polling for certificate updates and optional OCSP stapling.

## Usage

```go
provider, err := tlss3.New(
    tlss3.WithS3Client(s3Client),
    tlss3.WithBucket("certificates"),
    tlss3.WithCertificatePath("tls/server.crt"),
    tlss3.WithKeyPath("tls/server.key"),
    tlss3.WithPollInterval(5*time.Minute),
    tlss3.WithLogger(logger),
)
if err != nil {
    return err
}
defer provider.Close()

tlsConfig := provider.TLSConfig()
```

## Options

| Option | Description |
|--------|-------------|
| `WithS3Client` | S3 client for fetching certificates |
| `WithBucket` | S3 bucket containing certificates |
| `WithCertificatePath` | Object key for certificate file |
| `WithKeyPath` | Object key for private key file |
| `WithCAPath` | Optional object key for CA bundle |
| `WithPollInterval` | Certificate refresh interval (default: 5m) |
| `WithOCSPStapler` | Enable OCSP stapling |
| `WithLogger` | Logger for debug output |

## Key Types

| Type | Description |
|------|-------------|
| `S3` | S3-backed TLS certificate provider |

## Features

- Automatic certificate rotation via polling
- Support for mTLS with client CA bundle
- Optional OCSP stapling for better TLS performance
- Compatible with AWS S3, MinIO, and S3-compatible stores
- Thread-safe certificate updates without downtime