# s3

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
```

Package `s3` implements an OPA policy source backed by S3-compatible object storage. It lists and downloads `.rego` policy files from a configured
bucket and prefix.

A nonempty prefix selects a directory. Both `policies` and `policies/` list objects below `policies/`.

## Usage

```go
source, err := s3.New(
    s3.WithS3Client(s3Client),
    s3.WithBucket("my-policies"),
    s3.WithPrefix("opa/"),
    s3.WithReleaseManifest("release.json"),
)
if err != nil {
    return err
}
defer source.Close()

bundle, err := source.Fetch(ctx)
```

## Release layout

The example reads `s3://my-policies/opa/release.json`. Use `root` to select a release under `opa/` and `files` to map every included Rego module and
enabled JSON data key, relative to that root, to an object with a quoted S3 ETag and lowercase SHA-256 hex digest of the raw bytes. Copy the complete
ETag returned by S3, including the quotes encoded as part of the JSON string.

```json
{
  "root": "releases/2026-11-25",
  "files": {
    "authz.rego": {
      "etag": "\"0123456789abcdef0123456789abcdef\"",
      "sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
    }
  }
}
```

Each fetch reads the manifest once and lists only the selected directory prefix, such as `opa/releases/2026-11-25/`. The rooted form requires both
`root` and `files`. All paths use nonempty slash-separated segments. Absolute paths, `.` and `..` segments, backslashes, colons, NUL bytes, and
leading or trailing whitespace are rejected. Every policy and data GET uses the manifest's ETag in `If-Match` and verifies the SHA-256 digest. Keep
the manifest outside the release prefixes.

1. Upload the complete policy and data objects under a new, unused release prefix and record their ETags and digests. Publish the first complete
   release before starting the service.
2. Replace `opa/release.json` with a single PUT of the complete manifest.
3. Keep the old prefix unchanged until fetches that could have read its manifest have finished, then delete it.

A fetch that sees a missing, changed, or unlisted object fails and leaves the manager on its previous snapshot. Overwriting objects in an active prefix
can therefore cause a health check to report `NotServing`.

The flat JSON file map is also accepted for a fixed layout. Use the contents of `files` as the whole manifest object, with keys relative to the
configured prefix.

For a fixed release, `WithChecksums` supplies a map of relative keys to digests. GETs then use the ETags returned by the listing, and the fixed
digests still verify every object. Configure exactly one of the two options. YAML uses `opa.s3.releaseManifest` or `opa.s3.checksums`. A missing
manifest configuration returns `opa.ErrReleaseManifestRequired`.

## Options

| Option            | Description                                                                                                                     |
|-------------------|---------------------------------------------------------------------------------------------------------------------------------|
| `WithS3Client`    | S3 client (`S3API`: `ListObjectsV2` + `GetObject`; `*s3.Client` satisfies it) — required                                        |
| `WithBucket`      | S3 bucket name containing policies                                                                                              |
| `WithPrefix`      | Object key prefix for policy files                                                                                              |
| `WithExtensions`  | File extensions loaded as policies (default `DefaultExtensions`, `[".rego"]`)                                                   |
| `WithIncludeData` | Include `.json` data files in bundle                                                                                            |
| `WithLogger`      | Logger for debug output                                                                                                         |
| `WithReleaseManifest` | Manifest object key relative to the normalized prefix, read once per fetch |
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
- Each object is read through a 32 MiB cap.
