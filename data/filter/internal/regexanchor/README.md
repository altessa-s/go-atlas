# regexanchor

```go
import "github.com/altessa-s/go-atlas/data/filter/internal/regexanchor"
```

Rewrites the end-of-text anchors of an RE2 pattern so PCRE (MongoDB, MariaDB) and ICU (MySQL 8) read them as RE2 does. Internal to
`data/filter`; used by the MongoDB and MariaDB translators for `matches()`.

## Key functions

| Function                                      | Description                                                                                  |
|-----------------------------------------------|----------------------------------------------------------------------------------------------|
| `EndOfText(pattern string) (string, error)`   | Rewrites each `$` RE2 reads as end of text to `\z`; fails with `filter.ErrInvalidRegex` otherwise |

## Why

Outside multi-line mode RE2's `$` matches only at the very end; PCRE's and ICU's also match before a final newline, so `"abc\n"` matches
`abc$` there but not in CEL. `\z` is the absolute end in all three engines. A `$` in multi-line mode, an escaped `\$`, a `$` in a character
class and a `$` inside `\Q…\E` are left alone, and every rewrite is checked against RE2's own parse of the original pattern.
