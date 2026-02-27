# files

```go
import "github.com/altessa-s/go-atlas/core/io/files"
```

Package `files` provides utilities for file system checks and path resolution. All functions are pure, thread-safe, and follow symlinks.

## Functions

| Function              | Description                                                    |
|-----------------------|----------------------------------------------------------------|
| `FileExists`          | Check whether a path is an existing regular file               |
| `DirExists`           | Check whether a path is an existing directory                  |
| `DirIsEmpty`          | Check whether a directory contains no entries                  |
| `FindFile`            | Search for a file across multiple base directories             |
| `CurrentExecutableDir`| Resolve the directory of the running executable (symlink-safe) |
