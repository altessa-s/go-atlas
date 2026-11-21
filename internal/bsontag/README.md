# bsontag

```go
import "github.com/altessa-s/go-atlas/internal/bsontag"
```

Resolves BSON document keys and the tag options used by Atlas. The Atlas extension `omitonupdate` marks a field for omission from updates.

## API

| Function             | Result                                                                                 |
|----------------------|----------------------------------------------------------------------------------------|
| `Lookup(tag, key)`   | The configured tag value. Only the `bson` key accepts a bare tag without a colon.      |
| `Parse(tag, goName)` | The document name, skip flag, and `omitempty`, `omitonupdate`, `minsize`, `inline` flags. |

Only the complete tag `-` skips a field. An empty first token uses the lowercased Go field name. A nonempty first token sets the name and can also
select an option.
