# domain

The `domain` directory groups packages for domain-layer data transformation, validation, protobuf utilities, and in-process
coordination. These packages operate on plain Go structs and protobuf messages without depending on infrastructure concerns.

## Subpackages

| Package                        | Description                                          |
|--------------------------------|------------------------------------------------------|
| `behavior`                     | Struct-tag behavior strip/clean core + pluggable output `Translator` |
| `behavior/translators/mongo`   | MongoDB `bson.M` translator: `$set`/`$unset` documents + projections |
| `converter`                    | High-performance struct-to-struct conversion          |
| `converter/codec`              | Pluggable codec interface and chain execution engine  |
| `converter/codec/durpb`        | `durationpb.Duration` <-> `time.Duration` codec        |
| `converter/codec/jsonpb`       | `json.RawMessage` <-> `structpb.Struct` codec         |
| `converter/codec/mapslice`     | Map keys/values -> slice codec                        |
| `converter/codec/oneof`        | Struct <-> protobuf oneof codec                       |
| `converter/codec/pbwrap`       | Protobuf wrapper types <-> Go primitives codec        |
| `converter/codec/tspb`         | `timestamppb.Timestamp` <-> `time.Time`/`int64` codec |
| `converter/codec/unixtime`     | `time.Time` <-> `int64` Unix timestamp codec          |
| `converter/codecs/optionalcodec` | `core/types/optional.Optional[T]` field codec       |
| `eventbus`                     | Synchronous, lock-free, transaction-safe in-process event bus |
| `eventbus/uow`                 | In-process unit of work: post-commit effects with LIFO compensation |
| `fieldtracker`                 | Detect changed fields between two struct instances    |
| `normalizer`                   | Tag-based string normalization for struct fields      |
| `normalizer/modifiers`         | Built-in and custom modifier functions                |
| `proto/fieldbehavior`          | AIP-203 `field_behavior`-driven payload sanitization  |
| `proto/fieldmask`              | Hierarchical field mask utilities for protobuf        |
| `validation/iso7064`           | ISO 7064 MOD 11-10 check-digit validation            |
