# appinfo

```go
import "github.com/altessa-s/go-atlas/core/runtime/appinfo"
```

Package `appinfo` provides standardized access to application metadata, build information, environment variables, and directory paths.
Values are set via `-ldflags` at build time and accessed through package-level variables and helper functions.

## Build configuration

```bash
go build -ldflags "
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Name=MyApp'
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Version=1.2.3'
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Project=MyProject'
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Commit=abc123'
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'
  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.EnvPrefix=MYAPP'
"
```

## Globals

| Variable    | Description                                   | Default             |
|-------------|-----------------------------------------------|---------------------|
| `Name`      | Application name                              | Module path tail    |
| `Project`   | Umbrella project name                         | `""`                |
| `Version`   | Semantic version string                       | `"0.0.0"`           |
| `Commit`    | Abbreviated VCS revision (6 chars, uppercase) | From `vcs.revision` |
| `Branch`    | VCS branch name                               | `""`                |
| `BuildTime` | Build timestamp                               | From `vcs.time`     |
| `EnvPrefix` | Env var prefix (uppercased, `_`-separated)    | `""`                |

## Version checks

| Function             | Description                                          |
|----------------------|------------------------------------------------------|
| `IsRelease`          | Stable release (no pre-release identifiers)          |
| `IsPreRelease`       | Has pre-release identifiers                          |
| `IsAlpha`            | Version is `0.0.0` or has `alpha` pre-release        |
| `IsBeta`             | Has `beta` pre-release                               |
| `IsReleaseCandidate` | Has `rc` pre-release                                 |
| `SemVersion`         | Parsed `SemanticVersion` struct (shallow copy)       |

## Build info

| Function         | Description                                       |
|------------------|---------------------------------------------------|
| `AppVersion`     | Multi-line version summary for `--version`        |
| `Info`           | Compact one-liner: version, revision, prefix      |
| `BuildInfo`      | Compact one-liner: Go, platform, build time, tags |
| `BuildGoVersion` | Go toolchain version                              |
| `BuildPlatform`  | `GOOS/GOARCH`                                     |
| `BuildTags`      | Active build tags                                 |
| `Deps`           | Module dependencies                               |

## Environment variables

| Function        | Description                                 |
|-----------------|---------------------------------------------|
| `GetEnvVar`     | Read `<EnvPrefix>_<KEY>` with `~` expansion |
| `Env`           | Read env var (thin `os.Getenv` wrapper)     |
| `EnvOr`         | Read env var with a default                 |
| `EnvCached`     | Read env var with thread-safe caching       |
| `ClearEnvCache` | Invalidate all cached entries               |
| `HomeDir`       | User home directory (cross-platform)        |
| `ExpandPath`    | Replace leading `~` with home directory     |

## Directory paths

| Function        | Default path                 |
|-----------------|------------------------------|
| `BinDir`        | `/opt/bin`                   |
| `VarDir`        | `/var`                       |
| `EtcDir`        | `/etc/<project>/<name>`      |
| `LibDir`        | `/var/lib/<project>/<name>`  |
| `CertsCacheDir` | `<LibDir>/certs`             |
| `MakeAllDirs`   | Create `VarDir` and `LibDir` |

All paths are overridable via `<EnvPrefix>_*_DIR` environment variables. Paths are resolved once at first access and cached for the process lifetime.
