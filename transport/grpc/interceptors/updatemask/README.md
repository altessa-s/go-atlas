# updatemask

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/updatemask"
```

By default, updates use `update.Prepare` to validate raw paths, normalize the resource, write back the initial effective mask, and retain a deferred plan.
`IMMUTABLE` and `IDENTIFIER` values are captured in the deferred plan for comparison with stored state; their paths do not authorize writes in the initial mask.
Protected paths, including explicit clears, are deferred. An unchanged protected value is ignored; a change is `INVALID_ARGUMENT` from stateful
planning in the handler. `OUTPUT_ONLY` input is sanitized during normalization, including explicit empty and dropped-only masks.
An omitted mask implies populated fields; an explicit empty mask is a no-op. An empty interim mask can still require protected-field comparison.
With explicit paths or an explicit empty mask, the handler-visible resource retains selected input and the resource routing identity.
Unselected containers are dropped even when they contain IMMUTABLE or nested IDENTIFIER fields.
Selected collection elements retain their identifiers, map keys, and list positions; OUTPUT_ONLY input is cleared.
The plan captures original values for deferred comparison; selected protected values remain visible. Omitted masks and * retain protected input.
An omitted mask selects populated leaves; an explicit empty mask clears unselected mutable input and performs no writes or secret changes.
A dropped-only mask also filters unselected input. Writable masks retain selected values for downstream validation; `*` processes replacement selection.
`OUTPUT_ONLY` is cleared in every case. The deferred plan captures original input for comparison, replacements and selected `INPUT_ONLY` secrets.
No `KeepIdentity` cleanup runs before completion. Never persist the normalized request as a replacement.

The handler must call `update.Patch.Run` with the received context and stored-resource callbacks, or call `Spec.Plan` with
`update.WithPlanContext(ctx)` and `update.WithCurrent(stored)`, then merge the final paths. Merging only the written-back mask does not perform
the protected-field check. Always complete the plan, even when the written-back mask is empty.
Map handler errors with `errstatus.WithCanonicalRules()` (or equivalent conversion) to expose protected-field failures as `InvalidArgument`.

Unary handlers receive the plan through `update.ContextWithPlan`. Streaming handlers obtain a context snapshot from `stream.Context()` after
each successful receive. Each message gets its own plan; use a fresh resource instance when retaining earlier contexts. Both interceptors share
a per-message tracker: the first successful normalization and its original mask are retained, and the second interceptor reuses that plan.

The default extractor locates `update_mask`, falling back to `updateMask` only when the snake_case field is not declared.
`WithExtractor` overrides lookup; `WithMaskFieldName` names errors only. A lookup miss passes through unless strict mode is enabled.

## Unary validation scope

Only unary Update methods registered with `fieldmask.WithUpdateSpec` use mask-aware validation.
The mask interceptors resolve the original mask once and share the selection with the
factory's downstream Buf validator and the handler. An extractor miss or missing resource
uses the ordinary request validation path.

Register a handler-owned spec for a method when the service provides an allow-list or owns mask
error reporting:

```go
spec, err := update.NewSpec(new(pb.Settings),
    update.WithAllowedPaths("general", "general.time_zone"))
if err != nil {
    return err
}
builder.UseFieldMaskOptions(fieldmask.WithUpdateSpec("/example.v1.SettingsService/UpdateSettings", spec))
```

Register before adding the interceptors. A custom extractor should return the original mask,
including a nil mask when the client omitted it. For registered methods the framework strips output-only input
and leaves the sent mask and other values for handler-owned write planning.

Handler checklist

- Always check `selection.Err` before planning or writing.
- Use `selection.OriginalMask()` to preserve whole-message write intent.
`selection.Spec.ValidateResolved(selection.Resource, selection.Mask, update.WithCurrent(stored))`
checks the selected values against stored state and returns indexed violations. The handler can
use the same spec directly when invoked without an interceptor context.

For a resolved mask, Buf validates selected resource fields, routing identity, supplied
write-only values and selected map entries. AIP-202 format checks use the same scope.
Fields outside the resource retain their normal applicable rules. Empty required fields
of the resource report `required` with the full proto field path and collection subscripts.
For whole list elements and map entries, required checks follow populated nested messages.
An absent optional nested message is skipped unless the mask explicitly names a descendant.
An absent required nested message and its required descendants are reported for both collection types.

If a registered mask does not resolve, Buf validates only the request fields outside the
resource and the spec's resource identity fields. The handler receives the indexed error
in `selection.Err` and reports it before resource body validation. The field-behavior
interceptor still applies its usual AIP-202 format checks on retained input. If the
downstream validator is absent or ignores the RPC, the chain rejects the unresolved mask
before invoking the handler.

Unregistered RPCs and streaming RPCs use the deferred write plan and effective-mask writeback.

## Options

| Option                                | Default                   | Description                                                                         |
|---------------------------------------|---------------------------|-------------------------------------------------------------------------------------|
| `WithExtractor(fn)`                   | `extract.DefaultUpdate()` | How the `update_mask` and the resource are located.                                 |
| `WithUpdateOptions(...update.Option)` | go-aip defaults           | Options of `update.Prepare`: limits, path validation, field behavior resolution.  |
| `WithStrict()`                        | off                       | Refuse a missing or empty mask and check every path whatever the validation mode.   |
| `WithMaskFieldName(name)`             | `"update_mask"`           | The mask field named in strict errors.                                              |
