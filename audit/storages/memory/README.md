# memory

```go
import "github.com/altessa-s/go-atlas/audit/storages/memory"
```

Package `memory` provides an in-memory implementation of `audit.Storage`. All data lives in process memory and is lost on exit; use it in tests and
local development.

An empty batch succeeds. A nil event rejects the whole batch before any event is written.
