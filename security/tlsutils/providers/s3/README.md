# s3

```go
import tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
```

Package `tlss3` provides a TLS certificate provider that fetches certificates from S3-compatible storage. It supports automatic polling for certificate
updates and optional OCSP stapling.

## Usage

```go
provider, err := tlss3.New("certificates", "tls/server.crt", "tls/server.key", "", // bucket, cert key, private key key, key password
    tlss3.WithS3Client(s3Client),
    tlss3.WithPollInterval(5*time.Minute),
    tlss3.WithLogger(logger),
)
if err != nil {
    return err
}
defer provider.Close(ctx)

tlsConfig, err := provider.TLSConfig()
```

## Options

| Option | Description |
|--------|-------------|
| `WithS3Client` | S3 client (`S3API`) for fetching certificates (default: built from the AWS default config) |
| `WithPollInterval` | Certificate refresh interval (default: 5m) |
| `WithOcspStapler` | Enable OCSP stapling |
| `WithSseType` | Server-side encryption type: `SSETypeNone`, `SSETypeS3`, `SSETypeKMS`, `SSETypeC` |
| `WithSseCustomerKey` / `WithSseCustomerKeyMD5` | Customer-provided key and its MD5 for SSE-C |
| `WithReloadNotifyChan` | Channel signaled (non-blocking) after each certificate (re)load |
| `WithContext` | Base context for loading and polling (default: `context.Background()`) |
| `WithLogger` | Logger for debug output |

## Key Types

| Type | Description |
|------|-------------|
| `S3` | S3-backed TLS certificate provider |
| `S3API` | Subset of the S3 client API used by the provider; `*s3.Client` satisfies it |
| `SSEType` | Server-side encryption type for S3 objects |

## Features

- Automatic certificate rotation via polling (object ETags are compared before reloading)
- SSE-S3, SSE-KMS, and SSE-C server-side encryption
- Optional OCSP stapling for better TLS performance
- Compatible with AWS S3, MinIO, and S3-compatible stores
- Thread-safe certificate updates without downtime
