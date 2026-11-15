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

| Option            | Description                                                                                                                     |
|-------------------|---------------------------------------------------------------------------------------------------------------------------------|
| `WithS3Client`    | S3 client (`S3API`: `ListObjectsV2` + `GetObject`; `*s3.Client` satisfies it) — required                                        |
| `WithBucket`      | S3 bucket name containing policies                                                                                              |
| `WithPrefix`      | Object key prefix for policy files                                                                                              |
| `WithExtensions`  | File extensions loaded as policies (default `DefaultExtensions`, `[".rego"]`)                                                   |
| `WithIncludeData` | Include `.json` data files in bundle                                                                                            |
| `WithLogger`      | Logger for debug output                                                                                                         |
| `WithChecksums`   | Expected SHA-256 hex digest per object key relative to `WithPrefix`; every fetched object must match and every entry must exist |

## Key Types

| Type     | Description                            |
|----------|----------------------------------------|
| `Source` | S3-backed policy source implementation |

## Features

- Compatible with AWS S3, MinIO, and S3-compatible stores
- Efficient listing with prefix filtering
- Support for both policies (`.rego`) and data (`.json`) files
- Sequential download of policy files; any list or download error fails the fetch
- Retries with exponential backoff come from the injected S3 client's own retryer (the AWS SDK default)

## Security

- The transport is the injected S3 client's: keep its endpoint on `https://` (the AWS default). A custom plain-HTTP endpoint (for example a local
  MinIO) exposes the policies to tampering in transit: SigV4 protects the credentials, not the object content. The source cannot see an injected
  client's endpoint, so the [OPA factory](../../../../factory/auth/opa/) enforces it on the client it builds: a non-`https://` `s3.endpoint` fails
  with
  `ErrInsecureEndpoint` unless `s3.allowInsecureHTTP: true`.
- Pin the expected policies with `WithChecksums` (the same SHA-256 manifest as the filesystem source) so a writer to the bucket cannot swap them
  silently. Verification is optional: without `WithChecksums` the source loads whatever the bucket holds, and `New` logs a warning once
  (`s3 policy source has no checksums manifest`) through the `WithLogger` logger so an unverified source is never silent.
- Each object is read through a 32 MiB cap.
