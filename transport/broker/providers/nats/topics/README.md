# topics

```go
import "github.com/altessa-s/go-atlas/transport/broker/providers/nats/topics"
```

Package `topics` provides type-safe NATS subject templating. Define topic
templates with curly-brace macros like `{TENANT}` or `{ORDER_ID}` and
substitute concrete values at runtime. Subject sanitization, length, and
wildcard positioning are enforced by `Topic.WithValidation`.

## Key types

| Type       | Description                                                                                           |
|------------|-------------------------------------------------------------------------------------------------------|
| `Topic`    | NATS subject template containing curly-brace macro placeholders (e.g. `"events.{TENANT}"`)            |
| `TopicKey` | Named string used with `Topic.AcceptsMacros` and `Topic.RequiredMacros` for type-safe macro reference |

## Methods

| Method                 | Description                                                                                |
|------------------------|--------------------------------------------------------------------------------------------|
| `Topic.With`           | Substitute macros and return the subject. Panics on programmer error                       |
| `Topic.WithValidation` | Substitute macros and return `(string, error)`; enforces length and wildcard positioning   |
| `Topic.Validate`       | Validate the template structure (brace pairing, macro name characters, length)             |
| `Topic.AcceptsMacros`  | Report whether the template contains all the supplied macro keys                           |
| `Topic.RequiredMacros` | Return the sorted list of macro keys the template requires                                 |
| `TopicKey.String`      | Return the underlying string for use in `With` / `WithValidation` argument lists           |

## Constants

| Constant           | Value | Description                                                                  |
|--------------------|-------|------------------------------------------------------------------------------|
| `Wildcard`         | `"*"` | NATS single-token wildcard; preserved literally when used as a macro value   |
| `FullWildcard`     | `">"` | NATS multi-token wildcard; must be the last token of the subject              |
| `MaxSubjectLength` | `255` | NATS subject length limit enforced by `WithValidation`                       |

## Errors

| Error                        | Description                                                              |
|------------------------------|--------------------------------------------------------------------------|
| `ErrSubjectTooLong`          | Generated subject exceeds `MaxSubjectLength`                             |
| `ErrInvalidWildcardPosition` | `>` is not the last token of the subject                                 |
| `ErrEmptyMacroValue`         | Empty string supplied as a macro value                                   |
| `ErrInvalidCharacters`       | Macro value contains characters that cannot be used in a NATS subject    |

## Usage

```go
const tenantEvents topics.Topic = "events.{TENANT}"

subject := tenantEvents.With("TENANT", "acme")
// subject == "events.acme"

subject, err := tenantEvents.WithValidation("TENANT", externalInput)
if err != nil {
    return err
}
```

Wildcard substitution for subscription patterns:

```go
const tenantOrders topics.Topic = "orders.{TENANT}.{ORDER_ID}"

pattern := tenantOrders.With(
    "TENANT",   topics.Wildcard,
    "ORDER_ID", topics.FullWildcard,
)
// pattern == "orders.*.>"
```

Sanitization replaces NATS metacharacters in macro values with `_`, except for
the literal `Wildcard` and `FullWildcard` constants:

```go
tenantEvents.With("TENANT", "abc.123*def>ghi")
// == "events.abc_123_def_ghi"
```

## Concurrency

`Topic` and `TopicKey` are immutable strings; all methods are safe for
concurrent use. Parsed macros are cached in a process-global map keyed by
template, so the cache is bounded by the number of distinct templates declared.
