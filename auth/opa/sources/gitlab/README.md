# gitlab

```go
import "github.com/altessa-s/go-atlas/auth/opa/sources/gitlab"
```

Package `gitlab` provides an OPA policy source that reads policies from a GitLab repository.
It uses the GitLab API to fetch `.rego` policy files and optional `.json` data files.

## Usage

```go
source, err := gitlab.New(
    gitlab.WithEndpoint("https://gitlab.example.com"),
    gitlab.WithToken(token),
    gitlab.WithProjectID(42),
    gitlab.WithRef("main"),
    gitlab.WithDir("policies/opa"),
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
| `WithEndpoint` | GitLab API endpoint URL |
| `WithToken` | GitLab access token for authentication |
| `WithProjectID` | GitLab project ID containing policies |
| `WithRef` | Git ref (branch/tag/commit) to fetch from |
| `WithDir` | Directory path within the repository |
| `WithIncludeData` | Include `.json` data files in bundle |
| `WithHTTPClient` | Custom HTTP client for API requests |
| `WithLogger` | Logger for debug output |

## Key Types

| Type | Description |
|------|-------------|
| `Source` | GitLab-backed policy source implementation |

## Features

- Dynamic policy updates from GitLab repositories
- Support for both policies (`.rego`) and data (`.json`) files
- Configurable branch/tag/commit references
- Compatible with self-hosted GitLab instances
- Automatic retry and error handling