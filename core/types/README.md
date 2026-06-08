# types

Generic type utilities for the Atlas framework. Each subpackage can be imported independently with zero external dependencies.

## Subpackages

| Package                      | Description                                                                       |
|------------------------------|-----------------------------------------------------------------------------------|
| [bits](./bits)               | Generic bit manipulation for all integer types                                    |
| [constraints](./constraints) | Reusable generic type constraints                                                 |
| [nilcheck](./nilcheck)       | Deep nil checking for interfaces and reflect values                               |
| [optional](./optional)       | Generic `Optional[T]` for explicit "value or absent" semantics                    |
| [ptr](./ptr)                 | Pointer creation and safe dereferencing for primitives                            |
| [redacted](./redacted)       | `RedactedString` named type that masks sensitive fields in fmt/log/JSON/YAML/BSON |
| [result](./result)           | Generic `Result[T]` for one-of-(value, error) in channels, slices, maps           |
