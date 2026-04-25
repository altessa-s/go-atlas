# files

```go
import "github.com/altessa-s/go-atlas/core/io/files"
```

Package `files` provides utilities for file system checks, path resolution, and
directory iteration. All functions are pure, thread-safe, and follow symlinks
unless noted otherwise.

## Functions

| Function               | Description                                                    |
|------------------------|----------------------------------------------------------------|
| `FileExists`           | Check whether a path is an existing regular file               |
| `DirExists`            | Check whether a path is an existing directory                  |
| `DirIsEmpty`           | Check whether a directory contains no entries                  |
| `FindFile`             | Search for a file across multiple base directories             |
| `CurrentExecutableDir` | Resolve the directory of the running executable (symlink-safe) |
| `Walk`                 | Iterate directory entries with optional extension/type filters |

## Walk

`Walk` returns an `iter.Seq2[Entry, error]` over filesystem entries under a
root directory. By default it lists only the top-level contents and yields
every entry regardless of type. The behavior is configured via functional
options.

### Options

| Option                | Description                                              |
|-----------------------|----------------------------------------------------------|
| `WithExtensions`      | Filter by file extension (case-insensitive, leading dot optional) |
| `WithFileTypes`       | Filter by `FileType` (Regular, Dir, Symlink, Other)      |
| `WithRecursive`       | Recurse into subdirectories up to `DefaultMaxRecursionDepth` levels |
| `WithMaxDepth`        | Recurse with an explicit bound (0 = top-level only)      |
| `WithUnboundedDepth`  | Remove the recursion depth limit (use with care)         |
| `WithSkipHidden`      | Skip entries whose name begins with `.`                  |
| `WithFollowSymlinks`  | Follow symbolic links to directories during recursion    |

### Recursion depth

By default, `Walk` visits only the top-level contents of the root directory.
`WithRecursive()` enables recursion with a safety cap of
`DefaultMaxRecursionDepth` (256) levels — high enough for any realistic
filesystem layout, but bounded so a symlink cycle reached via
`WithFollowSymlinks` cannot run away. Use `WithMaxDepth(n)` to set an
explicit bound, or `WithUnboundedDepth()` to disable the cap entirely on
trees you fully trust.

### Example: list `.so` files in a flat directory

```go
for entry, err := range files.Walk("./plugins",
    files.WithExtensions(".so"),
    files.WithFileTypes(files.FileTypeRegular),
) {
    if err != nil {
        return err
    }
    fmt.Println(entry.Path)
}
```

### Example: recursively find YAML files, skipping hidden directories

```go
for entry, err := range files.Walk("/etc/app",
    files.WithRecursive(),
    files.WithExtensions(".yaml", ".yml"),
    files.WithFileTypes(files.FileTypeRegular),
    files.WithSkipHidden(),
) {
    if err != nil {
        log.Warn("walk error", "err", err)
        continue
    }
    process(entry.Path)
}
```

### Error semantics

Errors encountered while reading directories are yielded as a zero-value `Entry`
with a non-nil error. In recursive mode, errors from sub-directories do not
abort the walk — iteration continues with the next sibling. Consumers can stop
the iterator at any time by breaking out of the `range` loop.
