# reasoncode

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"
```

Maps protovalidate rule IDs to canonical, client-facing validation reason codes. A reason code is part of a service's public error contract, so it
must be a stable code rather than the raw, implementation-specific protovalidate rule ID (e.g. `int64.gte`). The standard rules are mapped here;
service-specific rules are supplied by the caller as a catalog. Any rule that resolves to no known code yields `UNKNOWN`, so a rule ID never reaches
the client.

## Canonical codes

| Constant | Value | Meaning |
|----------|-------|---------|
| `Unknown` | `UNKNOWN` | Rule has no mapping. |
| `InvalidMinLengthOrValue` | `INVALID_MIN_LENGTH_OR_VALUE` | Value or length below its minimum bound (`gte`, `gt`, `min_len`, `min_items`, …). |
| `InvalidMaxLengthOrValue` | `INVALID_MAX_LENGTH_OR_VALUE` | Value or length above (or not equal to) its bound (`lte`, `lt`, `max_len`, `len`, …). |
| `InvalidFormatEmail` | `INVALID_FORMAT_EMAIL` | Invalid email format. |
| `InvalidFormatUUID` | `INVALID_FORMAT_UUID` | Invalid UUID format. |
| `InvalidFormatRegex` | `INVALID_FORMAT_REGEX` | Value does not match a required pattern. |
| `InvalidFormatURL` | `INVALID_FORMAT_URL` | Invalid URL or URI format. |
| `InvalidEnumValue` | `INVALID_ENUM_VALUE` | Value outside the set allowed by an enum rule. |

A `required` violation is formatted as `{FIELD_NAME}_REQUIRED` (falling back to `REQUIRED` when the field name is unknown).

## Key types

| Type | Description |
|------|-------------|
| `Resolver` | Maps a rule ID to a canonical code, consulting a caller-supplied catalog before the built-in standard rules. |
| `Violation` | Interface the caller implements to adapt its concrete violation type, keeping this package free of any validation-library dependency. |

## Resolution

| Method | Use for |
|--------|---------|
| `Resolve(ruleID)` | Rules that depend on the rule ID alone. |
| `ResolveViolation(v)` | Rules that need context: `required` (field name) and combined numeric range rules (field value, to tell min from max). |

`NewResolver(catalog, catalogPrefixes...)` builds a resolver. A rule ID matching one of `catalogPrefixes` but absent from `catalog` resolves to
`Unknown` instead of falling through to the standard rules, so a caller's own rules cannot be mapped by accident.

## Usage

```go
resolver := reasoncode.NewResolver(myService.ReasonCodeCatalog, "acme.")

code := resolver.ResolveViolation(v) // e.g. "INVALID_MIN_LENGTH_OR_VALUE"
```

See [`../buf`](../buf) for the protovalidate integration that adapts buf violations to `Violation` and wires the resolver into the gRPC validator.
