# checksum

```go
import "github.com/altessa-s/go-atlas/auth/opa/internal/checksum"
```

Internal manifest verification shared by the [embed](../../sources/embed) and [filesystem](../../sources/filesystem) policy sources.
The manifest owns an immutable snapshot of its input and preserves each source's error identities.

## Key types

| Type       | Purpose                                            |
|------------|----------------------------------------------------|
| `Manifest` | SHA-256 verification and expected-file coverage     |

A nil manifest disables verification; an empty manifest rejects every policy file. `Verify` checks a loaded file and `Covered` detects missing files.
