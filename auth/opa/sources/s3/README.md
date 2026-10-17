# s3

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
```

Package `s3` implements an OPA policy source backed by S3-compatible object storage. It lists and downloads `.rego` policy files from a configured
bucket and prefix.

## Usage

```go
source, err := s3.New(
    s3.WithS3Client(s3Client),
    s3.WithBucket("my-policies"),
    s3.WithPrefix("opa/"),
)
if err != nil {
    return err
}
defer source.Close()

bundle, err := source.Fetch(ctx)
```

## Options

| Option | Description |
|--------|-------------|
| `WithS3Client` | S3 client (`S3API`: `ListObjectsV2` + `GetObject`; `*s3.Client` satisfies it) — required |
| `WithBucket` | S3 bucket name containing policies |
| `WithPrefix` | Object key prefix for policy files |
| `WithExtensions` | File extensions loaded as policies (default `DefaultExtensions`, `[".rego"]`) |
| `WithIncludeData` | Include `.json` data files in bundle |
| `WithLogger` | Logger for debug output |

## Key Types

| Type | Description |
|------|-------------|
| `Source` | S3-backed policy source implementation |

## Features

- Compatible with AWS S3, MinIO, and S3-compatible stores
- Efficient listing with prefix filtering
- Support for both policies (`.rego`) and data (`.json`) files
- Sequential download of policy files; any list or download error fails the fetch
- Retries with exponential backoff come from the injected S3 client's own retryer (the AWS SDK default)