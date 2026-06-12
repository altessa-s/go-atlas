# lua

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/lua"
```

Package `lua` translates filter AST nodes into Lua boolean expressions suitable for Redis `EVAL` scripts, so filters run server-side
on plain Redis keys.
