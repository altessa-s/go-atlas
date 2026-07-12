# proto

Protocol Buffers utilities for the Atlas framework domain layer. Field mask operations and `google.api.field_behavior`-driven payload
sanitization on protobuf messages, with well-known types handled transparently.

## Subpackages

| Package                          | Description                                                                                                              |
|----------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| [fieldmask](./fieldmask)         | Hierarchical field mask: filter, prune, union, intersection, update-mask validation. Reads `update_mask` from the request (AIP-134) and the read mask from either gRPC metadata (`x-goog-fieldmask`, AIP-157) or the deprecated `read_mask` request field (AIP-161). AIP-161 path grammar: backtick-quoted map keys (`reviews.` + "`John Smith`"), wildcard segments (`aliases.*.display_name`), and `InvalidArgument` on indexed access to repeated fields (`authors.0`) on the write path. |
| [fieldbehavior](./fieldbehavior) | Strip `OUTPUT_ONLY`, `IDENTIFIER`, `IMMUTABLE`, `INPUT_ONLY` fields from Create/Update requests and responses per AIP-203. |

## AIP coverage

| AIP                                          | Where it lives                                                                                    |
|----------------------------------------------|---------------------------------------------------------------------------------------------------|
| AIP-134 (Standard Update)                    | [`fieldmask`](./fieldmask) — `ApplyUpdateMask`, `ExtractUpdateMask`, writeback                    |
| AIP-157 (Partial responses) — request field  | [`fieldmask`](./fieldmask) — `ExtractReadMask` (AIP-161 marks the request field deprecated)        |
| AIP-157 (Partial responses) — side channel   | [`fieldmask`](./fieldmask) — `MetadataReadExtractor`, default header `x-goog-fieldmask`           |
| AIP-161 (Field masks — path grammar)         | [`fieldmask`](./fieldmask) — backtick-quoted map keys, `*` wildcard segments, indexed-access rejection on write          |
| AIP-203 (Field behavior)                     | [`fieldbehavior`](./fieldbehavior); [`fieldmask`](./fieldmask) enforces the same annotations during update-mask validation |
