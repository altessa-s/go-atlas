// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldViolation describes a single field-behavior constraint that was violated
// during [FieldMask.ApplyUpdateMask]. Path is the dot-separated path, Behavior
// is the google.api.field_behavior value that triggered the violation, and
// Reason explains the rejection in human-readable form.
//
// FieldViolation is an alias for [behavior.Violation] so the gRPC interceptor
// can render fieldmask and fieldbehavior violations through the same
// io.altessa.badrequest.v1.BadRequest FieldViolation mapping.
type FieldViolation = behavior.Violation

// UpdateMaskBehaviorError is returned by [FieldMask.ApplyUpdateMask] when one or
// more google.api.field_behavior annotations are violated (REQUIRED, IMMUTABLE,
// OUTPUT_ONLY, IDENTIFIER). Inspect Violations for per-field details.
//
// Distinct from [fieldbehavior.BehaviorViolationError]: this one carries
// per-field Reason strings produced by the update-mask validator, while the
// fieldbehavior variant aggregates strip-time violations without a reason.
type UpdateMaskBehaviorError struct {
	Violations []FieldViolation // One entry per violated field.
}

func (e *UpdateMaskBehaviorError) Error() string {
	var buf strings.Builder
	buf.WriteString("field behavior violation: ")

	for i, v := range e.Violations {
		if i > 0 {
			buf.WriteString("; ")
		}

		buf.WriteString(v.Path)
		buf.WriteString(": ")
		buf.WriteString(v.Reason)
	}

	return buf.String()
}

// ApplyUpdateMask applies the field mask for update operations:
//  1. With [WithPathValidation] set to [PathValidationEnforce], rejects paths absent
//     from the schema (fail-fast); [PathValidationWarn] reports them and continues
//  2. Validates field behaviors: REQUIRED, IMMUTABLE, OUTPUT_ONLY, IDENTIFIER (fail-fast)
//  3. Removes OUTPUT_ONLY fields from the mask, then adds the set IDENTIFIER
//     fields of the remaining branches, collection elements included, so
//     they survive the filter
//  4. Clears fields NOT in the mask
//  5. Sets default values for fields IN the mask but not populated
//
// With an empty mask the message itself is cleared, but the update is driven by
// the mask: an empty mask lists no fields and so updates nothing. This differs
// from an omitted update_mask, whose handling (passthrough, or synthesis from
// the resource's set fields) is decided by the caller before reaching this
// method.
//
// IDENTIFIER fields (AIP-203) are treated like IMMUTABLE: the identifier names
// the resource and must not be modified by an update. Including such a field
// in the mask produces a violation.
//
// Naming a singular message field whole (no sub-path) selects it for
// replacement or, when absent, clearing. When the message holds a
// server-managed field — OUTPUT_ONLY, IDENTIFIER or IMMUTABLE, directly or in
// a singular sub-message at any depth — the replacement would delete that
// field's stored value, so each such descendant is reported as a violation;
// list the message's other fields ("profile.display_name") instead.
//
// A repeated or map field named whole, or one of its elements named whole
// ("labels.k", "aliases.*"), is replaced as a whole and is not descended. A
// sub-path into the elements — through a map key ("labels.k.display_name"),
// the "*" wildcard ("aliases.*.display_name") or the implicit per-element
// form ("aliases.display_name") — is checked against the element message by
// its schema, not by the values present: an IDENTIFIER or IMMUTABLE leaf is a
// violation ("labels.k.id"), an OUTPUT_ONLY leaf is removed from the mask,
// and a singular message named whole inside an element follows the rule
// above. Each element such a sub-path addresses keeps its set IDENTIFIER
// fields, as a singular sub-message does: "labels.k.id" or "aliases.*.id"
// enters the mask so the filter does not clear it. List indexes
// ("aliases.0.id") are rejected with *ValidationError.
//
// Returns *ValidationError when [WithPathValidation] is [PathValidationEnforce] and
// a path does not exist in the schema, or *UpdateMaskBehaviorError if any field
// behavior constraints are violated.
//
// This ensures the service can distinguish:
//   - "don't update this field" (field not in mask)
//   - "clear this field" (field in mask, set to default)
//   - "update this field" (field in mask, set to value)
//
// Mutates the receiver: IDENTIFIER leaves of msg are added to msk and
// OUTPUT_ONLY entries are removed, so the cleaned msk reflects what was
// actually applied. Callers that want to reuse the original mask should
// pass [FieldMask.Clone] of it.
func (msk FieldMask) ApplyUpdateMask(msg proto.Message, opts ...ApplyOption) error {
	if msg == nil {
		return nil
	}

	descriptor := msg.ProtoReflect().Descriptor()
	paths := msk.ToPaths()

	if o := newApplyOptions(opts...); o.shouldValidatePaths() {
		for _, path := range paths {
			err := validatePath(descriptor, path)
			if err == nil {
				continue
			}
			if o.pathValidation == PathValidationEnforce {
				return err // fail-fast on the first schema-invalid path.
			}
			// PathValidationWarn: report every bad path but still apply the mask.
			// shouldValidatePaths guarantees a non-nil reporter here.
			if validationErr, ok := errors.AsType[*ValidationError](err); ok {
				o.pathReporter(validationErr)
			}
		}
	}

	for _, path := range paths {
		if err := rejectIndexedRepeatedAccess(descriptor, path); err != nil {
			return err
		}
	}

	var violations []FieldViolation
	msk.validateFieldBehaviors(msg, "", &violations)

	if len(violations) > 0 {
		return &UpdateMaskBehaviorError{Violations: violations}
	}

	// OUTPUT_ONLY entries go first, so a branch that named only such fields
	// is deleted before a set identifier could keep it alive.
	msk.removeOutputOnlyFields(msg)
	msk.preserveIdentifierFields(msg)
	msk.iterateFilter(msg)
	msk.setDefaultsForUnsetFields(msg)

	return nil
}

// preserveIdentifierFields adds set IDENTIFIER fields (AIP-203) to the mask as
// leaf entries so [FieldMask.iterateFilter] does not clear them. The identifier
// is the resource's routing key — callers normally pass it alongside the
// update_mask without listing it in the mask itself, and dropping it would
// silently break the update path. Only set fields are added: if the caller did
// not provide the identifier, this step is a no-op and downstream logic is
// untouched.
// An IDENTIFIER that is also OUTPUT_ONLY is not preserved: the update never
// writes an OUTPUT_ONLY field.
//
// The step descends every branch of the mask: singular sub-messages and the
// elements of repeated and map fields a sub-path addresses (see
// [preserveElementIdentifiers]), so an element keeps its identifier as a
// sub-message does. Leaves — whole messages, elements and collections — keep
// their whole value anyway and are not descended.
func (msk FieldMask) preserveIdentifierFields(msg proto.Message) {
	prf := msg.ProtoReflect()
	msk.preserveIdentifiers(prf, identifierFields(prf.Descriptor()))
}

// preserveIdentifiers is [FieldMask.preserveIdentifierFields] on prf, with
// ids the [identifierFields] of its type, resolved once by the caller for all
// the elements of a collection.
func (msk FieldMask) preserveIdentifiers(prf protoreflect.Message, ids []protoreflect.FieldDescriptor) {
	for _, fd := range ids {
		if name := string(fd.Name()); prf.Has(fd) {
			if _, exists := msk[name]; !exists {
				msk[name] = nil
			}
		}
	}
	// Recurse into the branches already in the mask so identifiers within
	// explicitly-updated subtrees are also preserved. Branches into
	// server-managed fields are gone by now: OUTPUT_ONLY ones were removed,
	// the others rejected.
	fields := prf.Descriptor().Fields()
	for name, nested := range msk {
		if nested == nil {
			continue
		}
		fd := fields.ByName(protoreflect.Name(name))
		if fd == nil || !prf.Has(fd) {
			continue
		}
		switch {
		case fd.IsList() || fd.IsMap():
			preserveElementIdentifiers(fd, prf.Get(fd), nested)
		case fd.Kind() == protoreflect.MessageKind && !isDynamicWellKnownType(fd):
			nested.preserveIdentifierFields(prf.Get(fd).Message().Interface())
		}
	}
}

// identifierFields returns the IDENTIFIER fields of md that
// [FieldMask.preserveIdentifierFields] keeps: those not also OUTPUT_ONLY.
func identifierFields(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	var ids []protoreflect.FieldDescriptor
	fields := md.Fields()
	for i := range fields.Len() {
		// OUTPUT_ONLY dominates: never preserved, even when it is also
		// IDENTIFIER, so its client value is cleared.
		if fd := fields.Get(i); serverManagedBehavior(fd) == annotations.FieldBehavior_IDENTIFIER {
			ids = append(ids, fd)
		}
	}
	return ids
}

// preserveElementIdentifiers runs [FieldMask.preserveIdentifierFields] on
// every element of the repeated or map field fd (value v) that its sub-mask
// nested addresses with a sub-path, using the element mask the filter applies
// to it: for a list the "*" sub-mask when present, else nested itself (the
// implicit per-element form); for a map the sub-mask of the entry's key, else
// the "*" one. Elements selected whole, or not at all, are skipped. The
// element mask is shared by the elements it addresses, so an identifier set
// on any of them is kept on all that carry one. Collections of scalars and of
// dynamic well-known types are skipped.
//
// The elements are not visited when no element mask can find an identifier
// (see [FieldMask.mayPreserve]), and a map without a "*" sub-mask is read by
// the keys the mask names instead of being scanned.
func preserveElementIdentifiers(fd protoreflect.FieldDescriptor, v protoreflect.Value, nested FieldMask) {
	elem := wildcardElementDescriptor(fd)
	if elem == nil {
		return
	}
	ids := identifierFields(elem)
	wildcard, hasWildcard := nested[WildcardSegment]
	if fd.IsList() {
		perElement := nested
		if hasWildcard {
			perElement = wildcard
		}
		if perElement == nil || !perElement.mayPreserve(elem, ids) {
			return
		}
		list := v.List()
		for i := range list.Len() {
			perElement.preserveIdentifiers(list.Get(i).Message(), ids)
		}
		return
	}
	mm := v.Map()
	if !hasWildcard {
		keyKind := fd.MapKey().Kind()
		for key, sub := range nested {
			if sub == nil || !sub.mayPreserve(elem, ids) {
				continue
			}
			// A key that does not parse back to itself matches no entry,
			// as in the filter, which compares the entry's key string.
			k, ok := parseMapKey(keyKind, key)
			if !ok {
				continue
			}
			if val := mm.Get(k); val.IsValid() {
				sub.preserveIdentifiers(val.Message(), ids)
			}
		}
		return
	}
	if !elementMasksMayPreserve(nested, elem, ids) {
		return
	}
	mm.Range(func(k protoreflect.MapKey, val protoreflect.Value) bool {
		sub, ok := nested[k.String()]
		if !ok {
			sub = wildcard
		}
		if sub != nil {
			sub.preserveIdentifiers(val.Message(), ids)
		}
		return true
	})
}

// mayPreserve reports whether [FieldMask.preserveIdentifiers] can add an
// entry anywhere in msk for a message of type md whose [identifierFields] are
// ids: md declares one, or a branch of msk reaches a singular message or
// collection whose element mask may. It follows the mask, not the schema, so
// it terminates on recursive types. For collections it over-approximates —
// every sub-mask is taken as an element mask — which only costs a visit that
// finds nothing.
func (msk FieldMask) mayPreserve(md protoreflect.MessageDescriptor, ids []protoreflect.FieldDescriptor) bool {
	if len(ids) > 0 {
		return true
	}
	fields := md.Fields()
	for name, nested := range msk {
		if nested == nil {
			continue
		}
		fd := fields.ByName(protoreflect.Name(name))
		switch {
		case fd == nil:
		case fd.IsList() || fd.IsMap():
			elem := wildcardElementDescriptor(fd)
			if elem == nil {
				continue
			}
			elemIDs := identifierFields(elem)
			if fd.IsList() && nested.mayPreserve(elem, elemIDs) {
				return true
			}
			if elementMasksMayPreserve(nested, elem, elemIDs) {
				return true
			}
		case fd.Kind() == protoreflect.MessageKind && !isDynamicWellKnownType(fd):
			if nested.mayPreserve(fd.Message(), identifierFields(fd.Message())) {
				return true
			}
		}
	}
	return false
}

// elementMasksMayPreserve reports whether any non-nil sub-mask of nested — the
// sub-mask of a collection whose element type is elem with [identifierFields]
// ids — may preserve an identifier (see [FieldMask.mayPreserve]).
func elementMasksMayPreserve(nested FieldMask, elem protoreflect.MessageDescriptor, ids []protoreflect.FieldDescriptor) bool {
	for _, sub := range nested {
		if sub != nil && sub.mayPreserve(elem, ids) {
			return true
		}
	}
	return false
}

// parseMapKey returns the map key of kind that a mask key names, and whether
// it names one: key must parse as kind and format back to itself, since the
// filter matches an entry by its key string ([protoreflect.MapKey.String]).
func parseMapKey(kind protoreflect.Kind, key string) (protoreflect.MapKey, bool) {
	var v protoreflect.Value
	switch kind { //nolint:exhaustive // the remaining kinds cannot be map keys.
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(key).MapKey(), true
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(key)
		if err != nil {
			return protoreflect.MapKey{}, false
		}
		v = protoreflect.ValueOfBool(b)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := strconv.ParseInt(key, 10, 32)
		if err != nil {
			return protoreflect.MapKey{}, false
		}
		v = protoreflect.ValueOfInt32(int32(n))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return protoreflect.MapKey{}, false
		}
		v = protoreflect.ValueOfInt64(n)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := strconv.ParseUint(key, 10, 32)
		if err != nil {
			return protoreflect.MapKey{}, false
		}
		v = protoreflect.ValueOfUint32(uint32(n))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			return protoreflect.MapKey{}, false
		}
		v = protoreflect.ValueOfUint64(n)
	default:
		return protoreflect.MapKey{}, false
	}
	k := v.MapKey()
	return k, k.String() == key
}

func (msk FieldMask) validateFieldBehaviors(
	msg proto.Message,
	prefix string,
	violations *[]FieldViolation,
) {
	prf := msg.ProtoReflect()
	fields := prf.Descriptor().Fields()

	// Iterate in sorted order so UpdateMaskBehaviorError.Violations is deterministic
	// across runs — Violations is rendered into io.altessa.badrequest.v1.BadRequest by the gRPC
	// interceptor and stable order matters for client diagnostics and tests.
	for _, fieldName := range slices.Sorted(maps.Keys(msk)) {
		nested := msk[fieldName]
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}

		fullPath := behavior.JoinPath(prefix, fieldName)
		isSet := prf.Has(fd)

		// Behaviors are checked by presence with a fixed precedence, not by
		// the first annotation: a field that is REQUIRED and IMMUTABLE (or
		// IDENTIFIER) is protected whatever the declaration order, and the
		// violation names the protecting behavior.
		switch {
		case behavior.Has(fd, annotations.FieldBehavior_OUTPUT_ONLY):
			// Dropped from the mask by removeOutputOnlyFields.
			continue

		case behavior.Has(fd, annotations.FieldBehavior_IDENTIFIER):
			// IDENTIFIER fields (AIP-203) name the resource and must not be
			// changed by an update — same constraint as IMMUTABLE in this path.
			*violations = append(*violations, FieldViolation{
				Path:     fullPath,
				Behavior: annotations.FieldBehavior_IDENTIFIER,
				Reason:   "identifier field cannot be modified",
			})
			continue

		case behavior.Has(fd, annotations.FieldBehavior_IMMUTABLE):
			*violations = append(*violations, FieldViolation{
				Path:     fullPath,
				Behavior: annotations.FieldBehavior_IMMUTABLE,
				Reason:   "immutable field cannot be modified",
			})
			continue

		case behavior.Has(fd, annotations.FieldBehavior_REQUIRED) && !isSet:
			*violations = append(*violations, FieldViolation{
				Path:     fullPath,
				Behavior: annotations.FieldBehavior_REQUIRED,
				Reason:   "required field cannot be cleared",
			})
			continue
		}

		if nested != nil && (fd.IsList() || fd.IsMap()) {
			// A sub-path into the elements: checked by the element schema.
			validateElementBehaviors(fd, nested, fullPath, violations)
			continue
		}
		if !isExpandableMessage(fd) {
			continue
		}
		if nested != nil {
			nested.validateFieldBehaviors(
				prf.Get(fd).Message().Interface(), fullPath, violations,
			)
			continue
		}
		// The whole message is selected: the update may replace or clear it,
		// and with it every field below. A protected descendant would go with
		// it, so the whole-field selection is rejected; the caller lists the
		// parent's other fields instead.
		protectedDescendants(fd.Message(), fullPath, map[protoreflect.FullName]bool{}, violations)
	}
}

// protectedDescendants reports every server-managed field — OUTPUT_ONLY,
// IDENTIFIER or IMMUTABLE — of md, and of its singular sub-messages at any
// depth, as a violation under prefix. It backs the rule that a message holding
// such a field cannot be named whole in an update mask: replacing or clearing
// the message would delete the field's stored value. Repeated and map fields
// are replaced as a whole by design and are not descended; an OUTPUT_ONLY
// sub-message is reported itself, not descended.
// visited breaks recursion on self-referential schemas: the fields of a
// message type already on the path are reported there.
func protectedDescendants(
	md protoreflect.MessageDescriptor,
	prefix string,
	visited map[protoreflect.FullName]bool,
	violations *[]FieldViolation,
) {
	visited[md.FullName()] = true
	defer delete(visited, md.FullName())

	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		path := behavior.JoinPath(prefix, string(fd.Name()))

		// Every server-managed field counts, also one that is REQUIRED:
		// replacing the parent would delete its stored value.
		switch serverManagedBehavior(fd) {
		case annotations.FieldBehavior_OUTPUT_ONLY:
			*violations = append(*violations, FieldViolation{
				Path:     path,
				Behavior: annotations.FieldBehavior_OUTPUT_ONLY,
				Reason:   "output-only field would be replaced with its parent; list the parent's other fields in the update mask",
			})
			continue
		case annotations.FieldBehavior_IDENTIFIER:
			*violations = append(*violations, FieldViolation{
				Path:     path,
				Behavior: annotations.FieldBehavior_IDENTIFIER,
				Reason:   "identifier field cannot be modified by replacing its parent; list the parent's other fields in the update mask",
			})
			continue
		case annotations.FieldBehavior_IMMUTABLE:
			*violations = append(*violations, FieldViolation{
				Path:     path,
				Behavior: annotations.FieldBehavior_IMMUTABLE,
				Reason:   "immutable field cannot be modified by replacing its parent; list the parent's other fields in the update mask",
			})
			continue
		default:
		}

		if isExpandableMessage(fd) && !visited[fd.Message().FullName()] {
			protectedDescendants(fd.Message(), path, visited, violations)
		}
	}
}

// hasProtectedDescendants reports whether [protectedDescendants] would report
// a violation for md: whether md holds a server-managed field directly or in a
// singular sub-message at any depth. visited breaks recursion on
// self-referential schemas.
func hasProtectedDescendants(md protoreflect.MessageDescriptor, visited map[protoreflect.FullName]bool) bool {
	visited[md.FullName()] = true
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if serverManagedBehavior(fd) != annotations.FieldBehavior_FIELD_BEHAVIOR_UNSPECIFIED {
			return true
		}
		if isExpandableMessage(fd) && !visited[fd.Message().FullName()] && hasProtectedDescendants(fd.Message(), visited) {
			return true
		}
	}
	return false
}

// serverManagedBehavior returns the server-managed behavior of fd that
// governs an update: OUTPUT_ONLY, then IDENTIFIER, then IMMUTABLE, whatever
// the declaration order, so OUTPUT_ONLY names a field that is also IDENTIFIER
// or IMMUTABLE. It returns FIELD_BEHAVIOR_UNSPECIFIED for any other field.
func serverManagedBehavior(fd protoreflect.FieldDescriptor) annotations.FieldBehavior {
	switch {
	case behavior.Has(fd, annotations.FieldBehavior_OUTPUT_ONLY):
		return annotations.FieldBehavior_OUTPUT_ONLY
	case behavior.Has(fd, annotations.FieldBehavior_IDENTIFIER):
		return annotations.FieldBehavior_IDENTIFIER
	case behavior.Has(fd, annotations.FieldBehavior_IMMUTABLE):
		return annotations.FieldBehavior_IMMUTABLE
	default:
		return annotations.FieldBehavior_FIELD_BEHAVIOR_UNSPECIFIED
	}
}

// validateElementBehaviors checks nested, the sub-mask of the repeated or map
// field fd, against the element message schema (see [elementMasks]).
// Collections of scalars and of dynamic well-known types have no element
// schema to check.
func validateElementBehaviors(
	fd protoreflect.FieldDescriptor,
	nested FieldMask,
	prefix string,
	violations *[]FieldViolation,
) {
	elem := wildcardElementDescriptor(fd)
	if elem == nil {
		return
	}
	for key, sub := range elementMasks(fd, nested) {
		sub.validateSchemaBehaviors(elem, elementPath(prefix, key), violations)
	}
}

// elementPath returns the path of the element sub-mask that [elementMasks]
// yields under key: prefix itself for the implicit list form (empty key).
func elementPath(prefix, key string) string {
	if key == "" {
		return prefix
	}
	return behavior.JoinPath(prefix, quotePathSegment(key))
}

// validateSchemaBehaviors is the descriptor-only counterpart of
// validateFieldBehaviors used inside collection elements, where the mask
// addresses every element (or those under a key) rather than one present
// value: IDENTIFIER and IMMUTABLE leaves are violations, OUTPUT_ONLY leaves
// are skipped (removeOutputOnlyFields drops them), a singular message named
// whole is checked with [protectedDescendants], and branches into singular
// messages and collections are descended. REQUIRED is not checked: it
// depends on the values present. Keys md does not declare ("*", indexes) are
// ignored.
func (msk FieldMask) validateSchemaBehaviors(
	md protoreflect.MessageDescriptor,
	prefix string,
	violations *[]FieldViolation,
) {
	fields := md.Fields()
	for _, name := range slices.Sorted(maps.Keys(msk)) {
		fd := fields.ByName(protoreflect.Name(name))
		if fd == nil {
			continue
		}
		nested := msk[name]
		path := behavior.JoinPath(prefix, name)

		switch serverManagedBehavior(fd) {
		case annotations.FieldBehavior_OUTPUT_ONLY:
			continue
		case annotations.FieldBehavior_IDENTIFIER:
			*violations = append(*violations, FieldViolation{
				Path:     path,
				Behavior: annotations.FieldBehavior_IDENTIFIER,
				Reason:   "identifier field cannot be modified",
			})
			continue
		case annotations.FieldBehavior_IMMUTABLE:
			*violations = append(*violations, FieldViolation{
				Path:     path,
				Behavior: annotations.FieldBehavior_IMMUTABLE,
				Reason:   "immutable field cannot be modified",
			})
			continue
		default:
		}

		switch {
		case nested != nil && (fd.IsList() || fd.IsMap()):
			validateElementBehaviors(fd, nested, path, violations)
		case !isExpandableMessage(fd):
		case nested != nil:
			nested.validateSchemaBehaviors(fd.Message(), path, violations)
		default:
			protectedDescendants(fd.Message(), path, map[protoreflect.FullName]bool{}, violations)
		}
	}
}

// elementMasks yields the parts of nested, the sub-mask of the repeated or
// map field fd, that address fields of its element messages, each with the
// mask key leading to it. For a list: first nested itself under the empty
// key — the implicit per-element form, whose keys other than "*" name element
// fields ("aliases.display_name") — then the "*" sub-mask. For a map: the
// sub-mask of every key, "*" included, in sorted order. A nil sub-mask names
// whole elements, which are replaced as a whole like the collection itself,
// and is not yielded. List indexes are not yielded: ApplyUpdateMask rejects
// them first.
func elementMasks(fd protoreflect.FieldDescriptor, nested FieldMask) iter.Seq2[string, FieldMask] {
	return func(yield func(string, FieldMask) bool) {
		if fd.IsList() {
			if !yield("", nested) {
				return
			}
			if sub := nested[WildcardSegment]; sub != nil {
				yield(WildcardSegment, sub)
			}
			return
		}
		for _, key := range slices.Sorted(maps.Keys(nested)) {
			if sub := nested[key]; sub != nil && !yield(key, sub) {
				return
			}
		}
	}
}

func (msk FieldMask) removeOutputOnlyFields(msg proto.Message) {
	if msg == nil {
		return
	}
	msk.stripOutputOnlyForDescriptor(msg.ProtoReflect().Descriptor())
}

// stripOutputOnlyForDescriptor removes OUTPUT_ONLY fields at the level of
// desc and recurses into the sub-masks of singular message fields
// ("sub.updated_at") and into the element sub-masks of repeated and map
// fields (see [elementMasks]: "aliases.*.updated_at", "aliases.updated_at",
// "labels.k.updated_at"), using the element message descriptor. A branch
// emptied by the removal is deleted, never left as a whole-message or
// whole-element selection. validateFieldBehaviors and
// validateSchemaBehaviors rely on this to skip OUTPUT_ONLY entries at any
// depth. Keys desc does not declare ("*", map keys) are skipped at this
// level.
func (msk FieldMask) stripOutputOnlyForDescriptor(desc protoreflect.MessageDescriptor) {
	if desc == nil {
		return
	}
	fields := desc.Fields()
	for fieldName, nested := range msk {
		if fieldName == WildcardSegment {
			continue
		}
		fd := fields.ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}
		if behavior.Has(fd, annotations.FieldBehavior_OUTPUT_ONLY) {
			delete(msk, fieldName)
			continue
		}
		if nested == nil {
			continue
		}
		if isExpandableMessage(fd) {
			// A branch into a singular sub-message: its OUTPUT_ONLY entries
			// go too, so validation can skip them. A branch left empty is
			// removed rather than turned into a whole-message selection.
			nested.stripOutputOnlyForDescriptor(fd.Message())
			if len(nested) == 0 {
				delete(msk, fieldName)
			}
			continue
		}
		if !fd.IsList() && !fd.IsMap() {
			continue
		}
		elemDesc := wildcardElementDescriptor(fd)
		if elemDesc == nil {
			continue
		}
		for key, sub := range elementMasks(fd, nested) {
			sub.stripOutputOnlyForDescriptor(elemDesc)
			if key != "" && len(sub) == 0 {
				delete(nested, key)
			}
		}
		if len(nested) == 0 {
			delete(msk, fieldName)
		}
	}
}

// wildcardElementDescriptor returns the message descriptor used as the
// recursion target for the "*" wildcard on fd: the list element message
// for repeated fields or the map value message for maps. Returns nil
// for scalar collections and dynamic well-known types whose subtree is
// not navigable by the schema.
func wildcardElementDescriptor(fd protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	switch {
	case fd.IsList():
		if fd.Kind() != protoreflect.MessageKind {
			return nil
		}
		m := fd.Message()
		if m == nil || isDynamicWellKnownMessage(m) {
			return nil
		}
		return m
	case fd.IsMap():
		v := fd.MapValue()
		if v == nil || v.Kind() != protoreflect.MessageKind {
			return nil
		}
		m := v.Message()
		if m == nil || isDynamicWellKnownMessage(m) {
			return nil
		}
		return m
	}
	return nil
}

func (msk FieldMask) setDefaultsForUnsetFields(msg proto.Message) {
	prf := msg.ProtoReflect()
	desc := prf.Descriptor()

	for fieldName, nested := range msk {
		fd := desc.Fields().ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue
		}

		if prf.Has(fd) {
			if nested != nil && fd.Kind() == protoreflect.MessageKind &&
				!fd.IsList() && !fd.IsMap() && !isDynamicWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(prf.Get(fd).Message().Interface())
			}
			continue
		}

		switch {
		case fd.IsList(), fd.IsMap():
			// AIP-134 distinguishes "clear this field" (in mask, absent on the
			// resource) from "leave alone" (not in mask). For maps prf.Mutable
			// already materializes a non-nil empty map; for lists we have to
			// poke the list to allocate a backing array, then truncate so the
			// observable state is "set, length 0" rather than "unset".
			mv := prf.Mutable(fd)
			if fd.IsList() {
				materializeEmptyList(mv.List())
			}
		case fd.Kind() == protoreflect.MessageKind:
			nestedMsg := prf.NewField(fd).Message()
			if nested != nil && !isDynamicWellKnownType(fd) {
				nested.setDefaultsForUnsetFields(nestedMsg.Interface())
			}
			prf.Set(fd, protoreflect.ValueOfMessage(nestedMsg))
		default:
			prf.Set(fd, fd.Default())
		}
	}
}

// materializeEmptyList forces list into a "set, length 0" state. protoreflect
// does not expose a direct "mark as set" primitive for repeated fields; the
// idiomatic workaround is to append a fresh element and immediately truncate.
// Used by setDefaultsForUnsetFields to honor the AIP-134 "clear this field"
// signal from the update mask.
func materializeEmptyList(list protoreflect.List) {
	list.Append(list.NewElement())
	list.Truncate(0)
}

// firstUpdateMaskBehavior collapses the google.api.field_behavior annotation
// list of fd into the single value that governs update-mask semantics:
// REQUIRED, IMMUTABLE, OUTPUT_ONLY, or IDENTIFIER. INPUT_ONLY, UNORDERED_LIST
// and NON_EMPTY_DEFAULT have no update-mask meaning and are reported as
// FIELD_BEHAVIOR_UNSPECIFIED so the caller treats them as optional.
func firstUpdateMaskBehavior(fd protoreflect.FieldDescriptor) annotations.FieldBehavior {
	for _, b := range behavior.Get(fd) {
		switch b { //nolint:exhaustive // INPUT_ONLY, UNORDERED_LIST, NON_EMPTY_DEFAULT have no update-mask semantics.
		case annotations.FieldBehavior_REQUIRED,
			annotations.FieldBehavior_IMMUTABLE,
			annotations.FieldBehavior_OUTPUT_ONLY,
			annotations.FieldBehavior_IDENTIFIER:
			return b
		}
	}

	return annotations.FieldBehavior_FIELD_BEHAVIOR_UNSPECIFIED
}
