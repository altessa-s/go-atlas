# proto

Protocol Buffers utilities for the Atlas framework domain layer. Provides field mask operations and `google.api.field_behavior`-driven
payload sanitisation on protobuf messages, with support for well-known types.

## Subpackages

| Package                          | Description                                                                                       |
|----------------------------------|---------------------------------------------------------------------------------------------------|
| [fieldmask](./fieldmask)         | Hierarchical field mask with filter, prune, union, intersection, and update-mask validation       |
| [fieldbehavior](./fieldbehavior) | Strip OUTPUT_ONLY, IDENTIFIER, IMMUTABLE, INPUT_ONLY fields from Create/Update requests and responses |
