# files

```go
import "github.com/altessa-s/go-atlas/core/io/files"
```

Package `files` provides utilities for file system checks and path resolution.

## Functions

| Function              | Description                                                    |
|-----------------------|----------------------------------------------------------------|
| `FileExists`          | Check whether a path is an existing regular file               |
| `DirExists`           | Check whether a path is an existing directory                  |
| `DirIsEmpty`          | Check whether a directory contains no entries                  |
| `FindFile`            | Search for a file across multiple base directories             |
| `CurrentExecutableDir`| Resolve the directory of the running executable (symlink-safe) |

## Usage

```go
if files.FileExists("/etc/app/config.yaml") {
    // load config
}

path := files.FindFile("config.yaml", []string{"config", "/etc/app"})

exeDir := files.CurrentExecutableDir()
```
