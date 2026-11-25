# retryqueuefields

```go
import "github.com/altessa-s/go-atlas/internal/retryqueuefields"
```

Document field names and flag-field validation shared by the MongoDB retry queue
and its configuration schema.

## Usage

| Function    | Purpose                                              |
|-------------|------------------------------------------------------|
| `ValidFlag` | Validates one flag name against its companion field. |
| `Reserved`  | Checks whether the queue owns a BSON field.          |
