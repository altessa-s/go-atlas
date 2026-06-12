// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"context"
	"fmt"
	"reflect"
)

// Object is a resolved struct: the ordered, instance-bound view of its exported
// fields produced by the engine's single reflection walk. It is the neutral
// intermediate representation a [Translator] folds into its output model.
type Object struct {
	Fields []Field
}

// Field is one resolved exported field within an [Object]. Translators read it to
// build their output; they decide representation, the engine decides traversal
// and policy (see Strip).
type Field struct {
	// Name is the Go field name (already promoted for embedded structs). It is
	// empty only for the synthetic field that wraps a nested-collection element
	// (a slice/array/map stored directly inside another collection).
	Name string
	// Tag is the field's full struct tag, so a translator can read whichever
	// secondary key it needs (bson, encryption, …) without the core knowing it.
	Tag reflect.StructTag
	// Kinds holds the field-behavior kinds declared in its behavior tag.
	Kinds []Kind
	// Value is the field value bound to the current instance. For a field reached
	// through a non-pointer map it is an addressable copy (see [Collection]).
	Value reflect.Value
	// Strip reports whether the field's kinds intersect the engine's configured
	// strip set ([WithKinds]). A stripped field is a leaf: the engine does not
	// descend into it (Nested/Collection stay nil).
	Strip bool
	// StripKind is the first of the field's kinds that matched the strip set,
	// valid only when Strip is true.
	StripKind Kind
	// Nested is the resolved sub-object for a struct or non-nil pointer-to-struct
	// field that was not itself stripped. Nil otherwise.
	Nested *Object
	// Collection holds the resolved elements for a slice, array, or map field
	// that was not itself stripped. Nil otherwise. It lives behind a pointer so
	// scalar and struct fields don't pay its size in the Fields slice — the
	// dominant allocation of the walk.
	Collection *Collection
}

// Collection is the resolved view of a slice, array, or map field's elements
// (see [Field.Collection]).
type Collection struct {
	// Items holds one resolved [Object] per element. A struct element resolves
	// to its own Object; an element that is itself a collection resolves to a
	// synthetic Object holding a single unnamed Field (see [Field.Name]) so
	// arbitrarily nested collections stay traversable; a nil-pointer or
	// non-struct element leaves an empty placeholder Object to keep Items
	// index-aligned.
	Items []Object
	// Keys are the map keys parallel to Items (map fields only); nil for
	// slices and arrays. A translator uses them to address the map entry behind
	// each Items element; the engine uses them for write-back and violation
	// paths.
	Keys []reflect.Value

	// roots are the addressable copies parallel to Items for a non-pointer
	// map; nil for pointer maps (whose entries are mutated through their shared
	// pointee) and for slices/arrays. The in-place fold writes each back with
	// SetMapIndex.
	roots []reflect.Value
}

// Translator folds the engine's resolved [Object] into an output model of type
// Out. Implementations must be safe for concurrent use: the engine may call
// Translate from multiple goroutines, so accumulate into a fresh value per call.
//
// Translate must not retain root — or any Object, Field, or Collection
// reachable from it — after it returns, and the returned Out must not alias
// that memory. Under [WithPooling] the engine recycles the tree's storage for
// later walks, so a retained reference would silently be overwritten with
// another value's data.
type Translator[Out any] interface {
	Translate(ctx context.Context, root Object) (Out, error)
}

// Engine drives the shared reflection walk and hands the result to a [Translator].
// Construct one with [New]; it is safe for concurrent use.
type Engine[Out any] struct {
	tr   Translator[Out]
	opts *options
}

// New builds an [Engine] that resolves structs under the given options and folds
// them with tr. The translator is injected here so the same core walk can feed
// different output models (a cleaned struct, a bson document, …).
func New[Out any](tr Translator[Out], opts ...Option) *Engine[Out] {
	o := newOptions(opts...)
	// A non-positive depth budget would reject even a flat struct; clamp it so
	// the root and its direct fields are always processed.
	o.maxDepth = max(1, o.maxDepth)
	return &Engine[Out]{tr: tr, opts: o}
}

// Translate resolves v and folds it with the engine's translator. v must be a
// struct or a non-nil pointer to a struct (a typed nil pointer is rejected);
// otherwise Translate returns an error and the zero Out.
func (e *Engine[Out]) Translate(ctx context.Context, v any) (Out, error) {
	if !e.opts.pooling {
		// The arena struct itself is never referenced from the tree, so it
		// stays on the stack; only its chunks go to the heap, owned by the GC.
		var a arena
		root, err := resolveValue(v, e.opts, &a)
		if err != nil {
			var zero Out
			return zero, err
		}
		return e.tr.Translate(ctx, root)
	}

	a := acquireArena()
	root, err := resolveValue(v, e.opts, a)
	if err != nil {
		releaseArena(a)
		var zero Out
		return zero, err
	}
	out, err := e.tr.Translate(ctx, root)
	// Deliberately not deferred: when the translator panics the tree may still
	// be referenced from the recovering frame, so the arena is abandoned to
	// the GC instead of being recycled.
	releaseArena(a)
	return out, err
}

// Resolve walks v and returns its resolved [Object] without folding it through a
// [Translator]. It is the building block a translator uses to recurse into
// values it discovers at run time — for example the struct elements of an []any
// or map[string]any. v must be a struct or a non-nil pointer to a struct.
func Resolve(v any, opts ...Option) (Object, error) {
	o := newOptions(opts...)
	o.maxDepth = max(1, o.maxDepth)
	// The resolved tree is handed to the caller, so it is built on a fresh,
	// never-pooled arena that the GC reclaims together with the tree.
	var a arena
	return resolveValue(v, o, &a)
}

// resolveValue validates v (struct or non-nil pointer-to-struct, dereferencing
// pointers) and resolves it under o.
func resolveValue(v any, o *options, a *arena) (Object, error) {
	if v == nil {
		return Object{}, fmt.Errorf("behavior: requires a struct or non-nil pointer to a struct, got nil")
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return Object{}, fmt.Errorf("behavior: requires a struct or non-nil pointer to a struct, got %T", v)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return Object{}, fmt.Errorf("behavior: requires a struct or pointer to a struct, got %s", rv.Kind())
	}

	return resolve(rv, o, a, 0)
}

// resolve walks the struct value sv and builds its [Object], binding instance
// values onto the per-type structural metadata. A field whose kinds intersect
// the strip set is marked Strip and left a leaf; otherwise the walk descends
// into structs, pointers, slices/arrays, and struct-valued maps.
func resolve(sv reflect.Value, o *options, a *arena, depth int) (Object, error) {
	if depth > o.maxDepth {
		return Object{}, ErrMaxDepthExceeded
	}

	infos, err := fieldsFor(sv.Type(), o.tagName)
	if err != nil {
		return Object{}, err
	}

	obj := Object{Fields: a.allocFields(len(infos))[:0]}
	for _, info := range infos {
		// A nil embedded pointer in the index path leaves promoted fields
		// unreachable; skip them rather than panic.
		fv, err := sv.FieldByIndexErr(info.index)
		if err != nil {
			continue
		}

		f := Field{Name: info.name, Tag: info.tag, Kinds: info.kinds, Value: fv}

		// The mask AND rejects non-matching fields cheaply; on a hit matchKind
		// is guaranteed to find the first intersecting kind.
		if info.kindMask&o.kindsMask != 0 {
			f.StripKind, _ = matchKind(info.kinds, o.kinds)
			f.Strip = true
			obj.Fields = append(obj.Fields, f)
			continue
		}

		if info.recurse {
			if err := descend(&f, fv, o, a, depth+1); err != nil {
				return Object{}, err
			}
		}

		obj.Fields = append(obj.Fields, f)
	}

	return obj, nil
}

// descend populates f.Nested or f.Collection from fv based on its runtime kind. Nil
// pointers and scalars are no-ops, except under schema-walk ([WithSchemaWalk]),
// where a nil pointer-to-struct descends a fresh zero of its element type so the
// resolved [Object] is type-complete.
func descend(f *Field, fv reflect.Value, o *options, a *arena, depth int) error {
	switch fv.Kind() {
	case reflect.Pointer:
		if fv.IsNil() {
			if o.schemaWalk {
				if elem := fv.Type().Elem(); structElemType(elem) != nil {
					return descend(f, reflect.New(elem).Elem(), o, a, depth)
				}
			}
			return nil
		}
		return descend(f, fv.Elem(), o, a, depth)

	case reflect.Struct:
		nested, err := resolve(fv, o, a, depth)
		if err != nil {
			return err
		}
		f.Nested = a.newObject(nested)
		return nil

	case reflect.Slice, reflect.Array:
		return descendList(f, fv, o, a, depth)

	case reflect.Map:
		return descendMap(f, fv, o, a, depth)

	default:
		return nil
	}
}

// structElemType unwraps pointer and collection layers behind t and returns the
// bottom struct type, or nil when none. Used by schema-walk to resolve a
// representative element from a collection's element type.
func structElemType(t reflect.Type) reflect.Type {
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		case reflect.Struct:
			return t
		default:
			return nil
		}
	}
}

// descendList resolves each element of a slice or array. Elements are
// addressable, so the in-place fold mutates them directly. A nil pointer element
// yields an empty [Object] placeholder to keep Items index-aligned. Under
// schema-walk ([WithSchemaWalk]) an empty slice/array yields one representative
// [Object] resolved from the bottom struct type behind the element type.
func descendList(f *Field, fv reflect.Value, o *options, a *arena, depth int) error {
	n := fv.Len()
	if n == 0 {
		if o.schemaWalk {
			if et := structElemType(fv.Type().Elem()); et != nil {
				child, err := resolve(reflect.New(et).Elem(), o, a, depth)
				if err != nil {
					return err
				}
				items := a.allocObjects(1)
				items[0] = child
				f.Collection = a.newCollection()
				f.Collection.Items = items
			}
		}
		return nil
	}

	items := a.allocObjects(n)
	for i := range n {
		child, err := resolveElement(fv.Index(i), o, a, depth)
		if err != nil {
			return err
		}
		items[i] = child
	}
	f.Collection = a.newCollection()
	f.Collection.Items = items

	return nil
}

// resolveElement resolves one collection element. Pointers are unwrapped, a
// struct resolves to its [Object], and an element that is itself a collection
// resolves to a synthetic Object holding a single unnamed [Field] bound to the
// element value, so arbitrarily nested collections ([][]T, map[K][]T, …) stay
// traversable. A nil pointer or non-struct leaf yields an empty placeholder
// Object so the caller's Items stay index-aligned.
func resolveElement(ev reflect.Value, o *options, a *arena, depth int) (Object, error) {
	for ev.Kind() == reflect.Pointer {
		if ev.IsNil() {
			return Object{}, nil
		}
		ev = ev.Elem()
	}

	switch ev.Kind() {
	case reflect.Struct:
		return resolve(ev, o, a, depth)
	case reflect.Slice, reflect.Array, reflect.Map:
		wrapper := Field{Value: ev}
		if err := descend(&wrapper, ev, o, a, depth); err != nil {
			return Object{}, err
		}
		fields := a.allocFields(1)
		fields[0] = wrapper
		return Object{Fields: fields}, nil
	default:
		return Object{}, nil
	}
}

// descendMap resolves each value of a map. Pointer-valued maps mutate their
// shared pointee in place; value-valued maps resolve over an addressable copy
// that the in-place fold writes back with SetMapIndex. Keys are recorded for
// both so violation paths can name the entry. Under schema-walk
// ([WithSchemaWalk]) an empty/nil map yields one representative [Object]
// resolved from the bottom struct type behind the value type (no key recorded).
func descendMap(f *Field, fv reflect.Value, o *options, a *arena, depth int) error {
	if fv.IsNil() || fv.Len() == 0 {
		if o.schemaWalk {
			if et := structElemType(fv.Type().Elem()); et != nil {
				child, err := resolve(reflect.New(et).Elem(), o, a, depth)
				if err != nil {
					return err
				}
				items := a.allocObjects(1)
				items[0] = child
				f.Collection = a.newCollection()
				f.Collection.Items = items
			}
		}
		return nil
	}

	byPointer := fv.Type().Elem().Kind() == reflect.Pointer
	n := fv.Len()
	c := a.newCollection()
	c.Items = a.allocObjects(n)[:0]
	c.Keys = a.allocKeys(n)[:0]
	if !byPointer {
		c.roots = a.allocKeys(n)[:0]
	}

	iter := fv.MapRange()
	for iter.Next() {
		k := iter.Key()
		val := iter.Value()

		if byPointer {
			c.Keys = append(c.Keys, k)
			child, err := resolveElement(val, o, a, depth)
			if err != nil {
				return err
			}
			c.Items = append(c.Items, child)
			continue
		}

		// Value map: map elements are not addressable, so resolve over a
		// writable copy and remember (key, root) for write-back.
		cp := reflect.New(val.Type()).Elem()
		cp.Set(val)
		child, err := resolveElement(cp, o, a, depth)
		if err != nil {
			return err
		}
		c.Items = append(c.Items, child)
		c.Keys = append(c.Keys, k)
		c.roots = append(c.roots, cp)
	}

	f.Collection = c
	return nil
}

// matchKind returns the first of a field's kinds that intersects the strip set
// want, and whether any intersection exists.
func matchKind(have, want []Kind) (Kind, bool) {
	for _, k := range have {
		for _, w := range want {
			if k == w {
				return k, true
			}
		}
	}
	return Unspecified, false
}

// joinPath joins prefix and name with a dot; an empty prefix yields name, and an
// empty name (a synthetic nested-collection wrapper) yields prefix.
func joinPath(prefix, name string) string {
	switch {
	case prefix == "":
		return name
	case name == "":
		return prefix
	default:
		return prefix + "." + name
	}
}

// mapKeyString renders a map key for a violation path.
func mapKeyString(k reflect.Value) string {
	if k.Kind() == reflect.String {
		return k.String()
	}
	return fmt.Sprintf("%v", k.Interface())
}
