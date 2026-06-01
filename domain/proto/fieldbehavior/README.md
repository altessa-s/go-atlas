# fieldbehavior

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
```

Package `fieldbehavior` strips fields from a `proto.Message` based on its `google.api.field_behavior` annotations (AIP-203). It solves a
different problem than `domain/proto/fieldmask`: where fieldmask validates that a `FieldMask` does not reference immutable or output-only
paths, fieldbehavior walks the payload itself and clears (or rejects) fields whose behavior makes them invalid for the current request
kind. Use it on the resource embedded in a Create/Update request before validation, and on a server response before returning it to the
caller, so server-managed identifiers, timestamps, and secrets never cross the wire in the wrong direction.

## Functions

| Function          | Default behaviors                              | Typical use                                 |
|-------------------|------------------------------------------------|---------------------------------------------|
| `StripCreate`     | `OUTPUT_ONLY`, `IDENTIFIER`                    | Create request payloads                     |
| `StripUpdate`     | `OUTPUT_ONLY`, `IDENTIFIER`, `IMMUTABLE`       | Update request payloads                     |
| `StripResponse`   | `INPUT_ONLY`                                   | Server responses (passwords, auth tokens)   |
| `Strip`           | none (no-op unless `WithBehaviors` is passed)  | Custom combinations                         |

## Options

| Option           | Default | Description                                                                                                  |
|------------------|---------|--------------------------------------------------------------------------------------------------------------|
| `WithBehaviors`  | n/a     | Override the behavior set. Passing it to a convenience function replaces (not extends) the default set.      |
| `WithStrict`     | off     | Return `*BehaviorViolationError` listing populated stripped fields instead of clearing them.                 |
| `WithMaxDepth`   | 32      | Cap on traversal depth. Bounds stack growth on fuzzed input; proto schemas may not contain cycles.           |

## Traversal semantics

| Field shape                                      | Behavior on field      | Behavior absent                                |
|--------------------------------------------------|------------------------|------------------------------------------------|
| Scalar, well-known type                          | Field cleared          | No-op                                          |
| Singular nested message                          | Whole subtree cleared  | Recursive descent into populated subtree       |
| Repeated of messages                             | Whole list cleared     | Recursive descent into each element            |
| Map with message values                          | Whole map cleared      | Recursive descent into each value              |
| Repeated/map of scalars                          | Field cleared          | No-op                                          |
| Oneof case                                       | Case cleared if set    | Recursive descent if the case is a message     |

Fields carrying multiple behaviors (for example `REQUIRED` + `IMMUTABLE`) match when at least one of their behaviors intersects the
configured set. `REQUIRED` and `UNORDERED_LIST` carry validation/semantic meaning rather than write-side restrictions and are not part of
any default set; pass them through `WithBehaviors` explicitly when needed.

## Example

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"

func (s *server) CreateBucket(ctx context.Context, req *pb.CreateBucketRequest) (*pb.Bucket, error) {
    if err := fieldbehavior.StripCreate(req.GetBucket()); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    // server-set fields (id, create_time) are now zero, safe to mint and validate.
    // ...
}

func (s *server) GetBucket(ctx context.Context, req *pb.GetBucketRequest) (*pb.Bucket, error) {
    b, err := s.repo.Get(ctx, req.GetId())
    if err != nil {
        return nil, err
    }
    // strip any INPUT_ONLY field the storage layer may have read back.
    if err := fieldbehavior.StripResponse(b); err != nil {
        return nil, err
    }
    return b, nil
}
```

## See also

- [`domain/proto/fieldmask`](../fieldmask/README.md) — hierarchical field mask filter, prune, and update-mask validation.
- AIP-203, AIP-133, AIP-134 — Google API Improvement Proposals defining `field_behavior` and the Create/Update conventions.
