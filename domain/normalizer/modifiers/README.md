# modifiers

```go
import "github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
```

Package `modifiers` provides value transformation functions for the `normalizer` package. Built-in
modifiers are registered at init time; custom modifiers can be added at runtime.

## Built-in modifiers

| Name                    | Description                                     |
|-------------------------|-------------------------------------------------|
| `trim`                  | Trim leading/trailing whitespace                |
| `lowercase`             | Convert to lowercase                            |
| `uppercase`             | Convert to uppercase                            |
| `nil_on_empty`          | Set `*string` to nil when empty                 |
| `phone`                 | Normalize phone numbers to E.164                |
| `remove_bad_symbols`    | Strip control chars and deprecated Unicode      |
| `remove_empty_elements` | Remove empty/nil entries from string slices     |

Both `RegisterModifier` and `GetModifier` are safe for concurrent use.
