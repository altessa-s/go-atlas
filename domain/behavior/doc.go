// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package behavior strips fields from a plain Go struct based on `behavior`
// struct tags, the reflection-based counterpart to
// [github.com/altessa-s/go-atlas/domain/proto/fieldbehavior].
//
// Where the proto package reads google.api.field_behavior annotations off a
// [proto.Message] descriptor, this package reads an equivalent vocabulary from
// `behavior:"…"` struct tags on an ordinary Go model, so a single domain struct
// can declare which of its fields are server-owned, immutable, or input-only —
// without a separate Create/Update/storage struct per operation.
//
// # Stripping
//
// "Strip" clears a field by setting it to its zero value. This works uniformly
// across field representations: a plain T becomes its zero value, a *T becomes
// nil, an [optional.Optional] becomes None, and a slice or map becomes nil — so
// callers are free to model partial updates with either *T or Optional and the
// outcome is the same.
//
// Three AIP-203 use cases are supported out of the box:
//
//   - [StripCreate] on a Create payload — clears OutputOnly and Identifier
//     fields the client should not be supplying.
//   - [StripUpdate] on an Update payload — additionally clears Immutable fields.
//   - [StripResponse] on a server response — clears InputOnly fields (secrets)
//     that must never leave the server.
//
// Use [Strip] with [WithKinds] for any custom combination. Pass [WithStrict]
// to surface populated stripped fields as a [*ViolationError] instead of
// silently clearing them. [Clean] is the copy counterpart of [Strip]: same
// clearing, but it returns a deep copy and leaves the original untouched.
//
// # Output translators
//
// [Strip]/[Clean] are the built-in "default" outputs (a cleaned struct). The
// package is structured as a core that performs a single reflection walk and
// hands the resolved tree to a pluggable [Translator], so the same walk can feed
// other output models without re-implementing traversal:
//
//	eng := behavior.New[bson.M](mongoTranslator, behavior.WithKinds(behavior.OutputOnly))
//	doc, err := eng.Translate(ctx, &bucket)
//
// [New] injects the translator; [Engine.Translate] resolves the struct into an
// [Object]/[Field] tree and folds it with the translator. behavior itself stays
// dependency-free; a database adapter (for example a mongo translator producing a
// bson update document) lives in its own package and is just one consumer.
//
// # Schema walk
//
// By default the walk binds instance values and stops where a nested value is
// absent (a nil pointer-to-struct, an empty slice/map of structs). A type-driven
// translator — one that enumerates the declared shape rather than the data, such
// as a query projection — needs those nested paths regardless of runtime
// contents. [WithSchemaWalk] makes the walk resolve nested struct types even when
// the value is absent: a nil pointer descends a fresh zero, and an empty
// collection contributes one representative element resolved from the bottom
// struct type behind its element/value type (unwrapping nested collection
// layers). The resulting [Object] is type-complete (one representative
// per collection) and each synthesized [Field.Value] is a non-addressable zero.
//
// Because synthesized values are fabricated, schema-walk is for read-only,
// type-driven translators only. [Strip], [Clean], and the in-place fold do not
// honor it (they fold real instance values); only [New]/[Engine.Translate] and
// [Resolve] thread it through.
//
// # Tags
//
// Annotate fields with one or more comma-separated kinds:
//
//	type Bucket struct {
//	    ID         string             `behavior:"identifier"`
//	    TenantID   string             `behavior:"immutable"`
//	    CreateTime time.Time          `behavior:"output_only"`
//	    Password   string             `behavior:"input_only"`
//	    Policy     optional.Optional[Policy]
//	}
//
//	// Reject server-owned fields supplied by the client.
//	if err := behavior.StripCreate(&bucket); err != nil {
//	    return err
//	}
//
// Tags on unexported fields are validated (a typo still fails loudly) but
// otherwise ignored: unexported fields cannot be set and never participate in
// a strip. Unlike encoding/json, name shadowing between an outer field and a
// field promoted from an embedded struct is not resolved — both fields are
// processed independently, and under [WithStrict] both report the same path.
package behavior
