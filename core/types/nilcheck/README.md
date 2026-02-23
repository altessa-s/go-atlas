# nilcheck

```go
import "github.com/altessa-s/go-atlas/core/types/nilcheck"
```

Package `nilcheck` provides deep nil checking that handles the Go interface-nil pitfall, where a non-nil interface can wrap a nil concrete value.

```go
var p *MyStruct = nil
var i any = p
i != nil          // true  (interface is not nil)
nilcheck.IsNil(i) // true  (underlying value is nil)
```

## Nil checks

| Function        | Description                                              |
|-----------------|----------------------------------------------------------|
| `IsNil`         | True if `nil` or wraps a nil pointer/map/slice/chan/func |
| `IsNotNil`      | Inverse of `IsNil`                                       |
| `IsNilValue`    | Same as `IsNil` for `reflect.Value` (avoids boxing)      |
| `IsNotNilValue` | Inverse of `IsNilValue`                                  |
| `IsEmptyValue`  | True if zero value, nil, or whitespace-only `*string`    |

## Validation

| Function        | Description                              |
|-----------------|------------------------------------------|
| `RequireNotNil` | Returns `*RequiredError` if value is nil |

## Checker

Fluent nil validation with error accumulation for constructors and factories.

```go
checker := nilcheck.NewChecker("MyFactory")
if err := checker.Check(db, "database").Check(cache, "cache").Error(); err != nil {
    return nil, err
}
```

| Method      | Description                    |
|-------------|--------------------------------|
| `Check`     | Validate non-nil, chainable    |
| `Error`     | First accumulated error or nil |
| `Errors`    | All accumulated errors         |
| `HasErrors` | True if any check failed       |
| `Reset`     | Clear errors for reuse         |

## Error types

| Type             | Description                              |
|------------------|------------------------------------------|
| `*RequiredError` | `"<FieldName> is required"` error value  |
