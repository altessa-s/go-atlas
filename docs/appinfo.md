# Application Info

Key concepts for working with `core/runtime/appinfo` — application metadata, version queries, environment variables, and filesystem paths.

```
import "github.com/altessa-s/go-atlas/core/runtime/appinfo"
```

---

## Overview

The `appinfo` package exposes application metadata populated at build time via `-ldflags` and at init time from `debug.ReadBuildInfo`. It provides:

- **Metadata variables** — name, project, version, commit, branch, build time
- **Version queries** — semantic version parsing, release/pre-release checks
- **Build info** — Go version, platform, build tags, CGO/race status
- **Environment variables** — prefixed lookups, caching, cross-platform home directory
- **Directory paths** — convention-based filesystem layout with env overrides
- **Standalone mode** — compile-time flag for deployment environment labeling

---

## Metadata variables

Set via linker flags during build:

```
go build -ldflags "\
  -X 'appinfo.Name=myservice' \
  -X 'appinfo.Project=myplatform' \
  -X 'appinfo.Version=1.2.3' \
  -X 'appinfo.BuildTime=2026-01-15T10:00:00Z' \
  -X 'appinfo.EnvPrefix=MYAPP' \
  -X 'appinfo.Commit=abc123' \
  -X 'appinfo.Branch=main'"
```

| Variable | Default | Description |
|----------|---------|-------------|
| `Name` | last path component of module, or `"unknown"` | Application/binary name |
| `Project` | `""` | Umbrella project name (groups related apps, used in directory paths) |
| `Version` | `"0.0.0"` | Semantic version string |
| `BuildTime` | `vcs.time` from build info | Build timestamp (RFC 3339) |
| `EnvPrefix` | `""` | Prefix for `GetEnvVar` lookups (uppercased, hyphens/dots replaced with `_`) |
| `Commit` | `vcs.revision` (6 chars, uppercased) | Abbreviated VCS revision hash |
| `Branch` | `""` | VCS branch name |
| `StandaloneLabel` | `"standalone"` | Label returned by `EnvLabel` when built with `standalone` tag |
| `NonStandaloneLabel` | `"cloud"` | Label returned by `EnvLabel` when built without `standalone` tag |

If `Name` is not set via ldflags, `init` derives it from the module path's last component. If build info is unavailable, it defaults to `"unknown"`.

If `Commit` or `BuildTime` are not set via ldflags, `init` reads them from `debug.ReadBuildInfo` settings (`vcs.revision`, `vcs.time`).

`EnvPrefix` is sanitized at init time: hyphens and dots are replaced with underscores, and the result is uppercased.

---

## Version queries

`Version` is parsed into a `SemanticVersion` at init time. The struct follows the SemVer 2.0.0 specification:

```go
type SemanticVersion struct {
    Major      string   // e.g. "1"
    Minor      string   // e.g. "2"
    Patch      string   // e.g. "3"
    Prerelease []string // e.g. ["alpha", "1"] for "1.2.3-alpha.1"
}
```

`SemVersion()` returns a copy (safe to modify):

```go
sv := appinfo.SemVersion()
fmt.Printf("major=%s minor=%s patch=%s\n", sv.Major, sv.Minor, sv.Patch)
```

### Query functions

| Function | Returns `true` when |
|----------|---------------------|
| `IsRelease()` | Stable release — no pre-release identifiers, not alpha, not beta |
| `IsPreRelease()` | Any pre-release identifiers present |
| `IsAlpha()` | Version is `"0.0.0"` or first pre-release segment is `"alpha"` |
| `IsBeta()` | First pre-release segment is `"beta"` |
| `IsReleaseCandidate()` | First pre-release segment is `"rc"` |

Example:

```go
if appinfo.IsAlpha() {
    logger.Warn("running alpha version", slog.String("version", appinfo.Version))
}
```

---

## Build info

Functions that return information about the build toolchain and target:

| Function | Returns |
|----------|---------|
| `Info()` | `"version=X.Y.Z, revision=ABCDEF, env_prefix=PREFIX"` |
| `BuildInfo()` | `"go=X.Y.Z, platform=os/arch, date=YYYY-MM-DD, tags=tag1,tag2"` |
| `AppVersion()` | Multi-line version banner (for `--version` CLI flags and startup banners) |
| `BuildGoVersion()` | Go toolchain version (e.g. `"go1.24.0"`) |
| `BuildPlatform()` | `"GOOS/GOARCH"` (e.g. `"linux/amd64"`) |
| `BuildGoOS()` | GOOS value (e.g. `"linux"`, `"darwin"`) |
| `BuildGoArch()` | GOARCH value (e.g. `"amd64"`, `"arm64"`) |
| `BuildTags()` | Comma-separated build tags |
| `IsCgoEnabled()` | Whether CGO was enabled at build time |
| `IsRaceEnabled()` | Whether the race detector was enabled |
| `EnvLabel()` | `StandaloneLabel` or `NonStandaloneLabel` based on build tag |

### Dependencies

`Deps()` returns the application's module dependencies as a `Dependencies` collection extracted from `debug.ReadBuildInfo`:

```go
deps := appinfo.Deps()

if deps.Contains("github.com/example/mod") {
    dep := deps.Get("github.com/example/mod")
    fmt.Printf("version=%s replaced=%v\n", dep.Version, dep.IsReplaced)
}
```

Each `Dependency` has: `Path`, `Version`, `Sum`, `IsReplaced`, and `ReplacedPath`.

---

## Environment variables

### Prefixed lookups

`GetEnvVar` prepends `EnvPrefix` (with `_` separator) to the key and looks up the resulting uppercase variable. A leading `~` in the value is expanded to
`HomeDir()`.

```go
// EnvPrefix = "MYAPP"
// GetEnvVar("config") looks up MYAPP_CONFIG
path := appinfo.GetEnvVar("config")
```

### General-purpose helpers

| Function | Description |
|----------|-------------|
| `Env(key)` | Thin wrapper around `os.Getenv` |
| `EnvOr(key, default)` | Returns `default` if variable is empty or unset |
| `EnvCached(key)` | Caches on first read (thread-safe); clear with `ClearEnvCache()` |

`EnvCached` uses double-checked locking (`sync.RWMutex`) to avoid re-reading the environment on hot paths. Call `ClearEnvCache()` in tests or after
modifying environment variables.

### Home directory

| Function | Description |
|----------|-------------|
| `HomeDir()` | Cross-platform home directory (`HOME`, `USERPROFILE`, `HOMEDRIVE`+`HOMEPATH`) |
| `ExpandPath(path)` | Replaces leading `~` with `HomeDir()` |

### Well-known constants

The package defines constants for commonly used environment variable names:

| Constant | Value |
|----------|-------|
| `EnvHome` | `"HOME"` |
| `EnvUserProfile` | `"USERPROFILE"` |
| `EnvHomeDrive` | `"HOMEDRIVE"` |
| `EnvHomePath` | `"HOMEPATH"` |
| `EnvNATSURL` | `"NATS_URL"` |
| `EnvAnthropicAPIKey` | `"ANTHROPIC_API_KEY"` |
| `EnvVaultToken` | `"VAULT_TOKEN"` |

---

## Directory paths

Convention-based filesystem layout for application binaries, configuration, and state. All directory functions check for an `<EnvPrefix>_*_DIR` environment
variable override before falling back to defaults.

| Function | Override var | Default | Description |
|----------|-------------|---------|-------------|
| `BinDir()` | `<PREFIX>_BIN_DIR` | `/opt/bin` | Application binaries |
| `VarDir()` | `<PREFIX>_VAR_DIR` | `/var` | Variable runtime data |
| `EtcDir()` | `<PREFIX>_ETC_DIR` | `/etc/<Project>/<Name>` | Configuration files |
| `LibDir()` | `<PREFIX>_LIB_DIR` | `<VarDir>/lib/<Project>/<Name>` | Library and state files |
| `CertsCacheDir()` | — | `<LibDir>/certs` | Cached TLS certificates |

`Project` and `Name` are lowercased in path construction. If `Project` is empty, the project component is omitted.

### Creating directories

```go
if err := appinfo.MakeAllDirs(); err != nil {
    log.Fatal("failed to create app directories", err)
}
```

`MakeAllDirs()` creates `VarDir` and `LibDir` (and any necessary parents) using `os.MkdirAll` with `0777` permissions (process umask applies).

### Example layout

With `Project = "myplatform"` and `Name = "myservice"`:

```
/etc/myplatform/myservice/     ← EtcDir()
/var/lib/myplatform/myservice/ ← LibDir()
/var/                          ← VarDir()
/opt/bin/                      ← BinDir()
```

---

## Standalone mode

The `IsStandalone` variable is set at compile time via the `standalone` build tag:

```
go build -tags standalone
```

| Build | `IsStandalone` | `EnvLabel()` |
|-------|----------------|--------------|
| `go build` | `false` | `"cloud"` |
| `go build -tags standalone` | `true` | `"standalone"` |

Both `StandaloneLabel` and `NonStandaloneLabel` can be overridden via ldflags if custom labels are needed.

---

## See also

- [runtime.md](runtime.md) — shutdown hooks, resource cleanup, `core/runtime` package map
- [concurrency.md](concurrency.md) — batch processing, concurrency limits, retry,
  panic recovery, signal handling
