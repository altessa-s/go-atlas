# filesystem

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
```

Package `filesystem` loads Rego modules and optional JSON data from a directory. It verifies each fetch against a complete release manifest before
returning the bundle to the [OPA manager](../../).

## Usage

```go
source, err := filesystem.New("/etc/policies",
    filesystem.WithReleaseManifest("release.json"),
    filesystem.WithIncludeData(),
)
```

## Release layout

With this configuration, `/etc/policies/release.json` can select a release directory under `/etc/policies`. Use `root` for its slash-separated relative
path and `files` to map every included Rego module and enabled JSON data file, relative to that root, to the lowercase SHA-256 hex digest of its raw
bytes.

```json
{
  "root": "releases/2026-11-25",
  "files": { "authz.rego": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" }
}
```

Each fetch reads the manifest once, opens the selected directory, and verifies coverage inside that directory. The rooted form requires both `root`
and `files`. All paths use nonempty slash-separated segments. Absolute paths, `.` and `..` segments, backslashes, colons, NUL bytes, and leading or
trailing whitespace are rejected. Relative symlinks must resolve within the configured directory. Keep the manifest outside the release directories.

1. Write the complete policy and data files under a new, unused release root and compute their digests. Publish the first complete release before
   starting the service.
2. Write the manifest to a temporary file on the same filesystem and atomically rename it over `/etc/policies/release.json`.
3. Keep the old root unchanged until fetches that could have read its manifest have finished, then delete it.

A fetch that sees a missing, changed, or unlisted file fails and leaves the manager on its previous snapshot. Overwriting files in an active root can
therefore cause a health check to report `NotServing`.

The flat JSON file map is also accepted for a fixed layout. Use the contents of `files` as the whole manifest object, with paths relative to the
configured directory. For a single `.rego` file passed to `New`, use the flat form in its parent directory and list the file by basename.

For a fixed release, Go callers can supply the same map with `WithChecksums`. Configure exactly one manifest option. The factory uses the
`opa.releaseManifest` YAML key. A missing manifest configuration returns `opa.ErrReleaseManifestRequired`.

## Options

| Option                | Default     | Description                                            |
|-----------------------|-------------|--------------------------------------------------------|
| `WithReleaseManifest` | unset       | Relative path to the JSON manifest read on every fetch |
| `WithChecksums`       | nil         | Static map of relative paths to SHA-256 hex digests    |
| `WithExtensions`      | `[".rego"]` | File extensions to load as policy modules              |
| `WithIncludeData`     | false       | Load `.json` files as OPA data                          |
| `WithLogger`          | discard     | Structured logger (`*slog.Logger`)                     |
