# mapslice

```go
import "github.com/altessa-s/go-atlas/domain/converter/codec/mapslice"
```

Package `mapslice` provides codecs for converting map values or keys to slices. Two package-level codecs
are available.

## Codecs

| Codec    | Description                                  |
|----------|----------------------------------------------|
| `Values` | Extract map values into a destination slice  |
| `Keys`   | Extract map keys into a destination slice    |

Element types must match or the destination must be `[]any`. Iteration order follows Go map semantics
(non-deterministic).
