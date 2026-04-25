# fieldmask

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
```

Package `fieldmask` provides hierarchical field mask utilities for Protocol Buffers. A `FieldMask` is a nested map representing
dot-separated field paths with support for filtering, pruning, set operations, and update-style validation of field annotations.
Well-known types (`structpb.Struct`, `structpb.ListValue`, `structpb.Value`) are handled transparently — their dynamic keys are
navigable by all mask operations. Update masks validate `google.api.field_behavior` annotations (`REQUIRED`, `IMMUTABLE`,
`OUTPUT_ONLY`) before applying changes.

## Constructors

| Function               | Description                                                                                            |
|------------------------|--------------------------------------------------------------------------------------------------------|
| `FromPaths`            | Create a mask from variadic dot-separated path strings                                                 |
| `FromProtoFieldMask`   | Create a mask from a `fieldmaskpb.FieldMask` proto message                                            |
| `FromMessage`          | Create a mask covering every field in the message schema                                               |
| `FromSetFields`        | Create a mask covering only the populated (non-default) fields of a message                            |

## Operations

| Method                 | Description                                                                                            |
|------------------------|--------------------------------------------------------------------------------------------------------|
| `Filter`               | Keep only the fields present in the mask, clearing everything else                                     |
| `Prune`                | Remove the fields present in the mask, keeping everything else                                         |
| `Union`                | Combine two masks into one that covers both sets of paths                                              |
| `Intersection`         | Return a mask containing only the paths present in both masks                                          |
| `Difference`           | Return a mask containing paths in the receiver but not in the argument                                 |
| `ToPaths`              | Return a sorted slice of dot-separated path strings                                                    |
| `ApplyUpdateMask`      | Copy masked fields from source to destination with field behavior validation                           |
