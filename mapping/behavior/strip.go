// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"fmt"
	"reflect"
	"strconv"
	"unsafe"
)

// Strip clears every field of *v whose `behavior` tag intersects the configured
// behavior set (see [WithKinds]). With no [WithKinds] passed, Strip is a no-op.
//
// Strip is the in-place "default" output: a plain T becomes its zero value, a *T
// becomes nil, an [optional.Optional] becomes None, and a slice or map becomes
// nil — so callers may model partial updates with either *T or Optional and the
// outcome is the same. It descends into nested structs, pointers to structs,
// and slices/arrays/maps — through arbitrarily nested collection layers
// ([][]T, map[string][]T, …) down to their struct elements — and promotes
// fields of embedded (anonymous) structs to the parent. A tagged field is
// cleared whole without further descent. Opaque structs (no exported fields,
// such as time.Time or [optional.Optional]) and interface-typed fields are
// leaves.
//
// Under [WithStrict] Strip leaves v untouched and returns *[ViolationError]
// listing every populated field that would have been cleared. With strict
// disabled (the default) Strip clears fields in place and returns nil unless
// [WithMaxDepth] is exceeded ([ErrMaxDepthExceeded]).
//
// A nil v is a no-op. Strip mutates v in place and is not safe to call
// concurrently on the same value; concurrent calls on distinct values are safe.
func Strip[S any](v *S, opts ...Option) error {
	return strip(v, newOptions(opts...))
}

// strip is the resolved-options core of [Strip], shared with the
// StripCreate/StripUpdate/StripResponse helpers so they can preconfigure the
// strip set without building a combined Option slice per call.
func strip[S any](v *S, o *options) error {
	if len(o.kinds) == 0 || v == nil {
		return nil
	}
	// A non-positive depth budget would reject even a flat struct; clamp it so
	// the root and its direct fields are always processed.
	o.maxDepth = max(1, o.maxDepth)
	// Schema-walk synthesizes representative values for absent nested structs;
	// the in-place fold would then try to clear non-addressable zeros. Strip
	// (and Clean, which delegates here) fold real instance values only.
	o.schemaWalk = false

	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("behavior: Strip requires a pointer to a struct, got pointer to %s", rv.Kind())
	}

	// The resolved tree never escapes Strip — the folds emit only zero writes
	// and path strings — so it is built on a pooled arena and recycled on
	// every non-panicking path.
	a := acquireArena()
	root, err := resolve(rv, o, a, 0)
	if err != nil {
		releaseArena(a)
		return err
	}

	if o.strict {
		var violations []Violation
		foldViolations(root, "", &violations)
		releaseArena(a)
		if len(violations) > 0 {
			return &ViolationError{Violations: violations}
		}
		return nil
	}

	foldInPlace(root)
	releaseArena(a)
	return nil
}

// StripCreate clears fields marked OutputOnly or Identifier. Use it on a Create
// payload before validation. The default set is applied first; a caller's
// [WithKinds] overrides it entirely.
func StripCreate[S any](v *S, opts ...Option) error {
	return strip(v, stripOptions(DefaultCreateKinds, opts))
}

// StripUpdate clears fields marked OutputOnly or Immutable. Use it on an Update
// payload before applying the update. Identifier fields are kept: on update the
// identifier addresses the resource (AIP-203), so it is neither cleared nor
// reported under [WithStrict].
func StripUpdate[S any](v *S, opts ...Option) error {
	return strip(v, stripOptions(DefaultUpdateKinds, opts))
}

// StripResponse clears fields marked InputOnly. Use it on a server response
// before returning it so secrets (passwords, tokens) never leave the server.
func StripResponse[S any](v *S, opts ...Option) error {
	return strip(v, stripOptions(DefaultResponseKinds, opts))
}

// stripOptions resolves options with base as the initial strip set and the
// caller's opts applied on top, so a caller's [WithKinds] still overrides the
// default entirely — without allocating a wrapper option and a concatenated
// slice on every call.
func stripOptions(base []Kind, opts []Option) *options {
	o := defaultOptions()
	o.setKinds(base)
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Clean returns a deep copy of v with every field whose `behavior` tag intersects
// the configured behavior set cleared, leaving the original untouched. It is the
// copy counterpart of [Strip]: identical traversal and clearing semantics, but
// the caller's value is never mutated. With no [WithKinds] (or an empty set)
// Clean returns a shallow copy unchanged.
//
// Only exported fields (and unexported embedded structs, whose exported fields are promoted) are deep-copied; other unexported
// fields are copied shallowly. Since [Strip] never touches them this preserves the original, but the returned copy shares any such
// unexported reference-typed fields with v. A value nested deeper than [WithMaxDepth], including a cyclic one, fails with
// [ErrMaxDepthExceeded].
// Interface-typed fields are likewise shared: they are leaves of the walk
// (stripping one clears the field itself, never the pointee), so the original
// stays intact while the copy avoids a needless deep clone.
func Clean[S any](v S, opts ...Option) (S, error) {
	o := newOptions(opts...)
	if len(o.kinds) == 0 {
		return v, nil
	}

	cp := v
	o.maxDepth = max(1, o.maxDepth)
	c := cloner{o: o}
	if err := c.cloneValue(reflect.ValueOf(&cp).Elem(), 0); err != nil {
		return v, err
	}
	if err := Strip(&cp, opts...); err != nil {
		return v, err
	}
	return cp, nil
}

// foldInPlace clears stripped fields of obj and descends into the rest, writing
// non-pointer map entries back with SetMapIndex.
func foldInPlace(obj Object) {
	for i := range obj.Fields {
		f := &obj.Fields[i]
		if f.Strip {
			f.Value.SetZero()
			continue
		}
		if f.Nested != nil {
			foldInPlace(*f.Nested)
		}
		if c := f.Collection; c != nil {
			for j := range c.Items {
				foldInPlace(c.Items[j])
				if c.roots != nil {
					f.Value.SetMapIndex(c.Keys[j], c.roots[j])
				}
			}
		}
	}
}

// foldViolations records every populated stripped field of obj as a [Violation],
// building dot/index/key paths, without mutating anything.
func foldViolations(obj Object, prefix string, out *[]Violation) {
	for i := range obj.Fields {
		f := &obj.Fields[i]
		path := joinPath(prefix, f.Name)

		if f.Strip {
			if !f.Value.IsZero() {
				*out = append(*out, Violation{Path: path, Kind: f.StripKind})
			}
			continue
		}

		if f.Nested != nil {
			foldViolations(*f.Nested, path, out)
		}
		if c := f.Collection; c != nil {
			for j := range c.Items {
				var child string
				if c.Keys != nil {
					child = path + "[" + strconv.Quote(mapKeyString(c.Keys[j])) + "]"
				} else {
					child = path + "[" + strconv.Itoa(j) + "]"
				}
				foldViolations(c.Items[j], child, out)
			}
		}
	}
}

// cloner deep-copies the data [Strip] can reach, following the same plan as the strip walk: only fields the walk descends into are
// copied, a field cleared whole is not descended, promoted fields of embedded structs cost no depth, and every nested struct costs
// the depth [resolve] charges. Pointers, maps and slices are memoized by source identity, so shared nodes stay shared in the clone
// and a cycle terminates.
type cloner struct {
	o    *options
	seen map[cloneKey]reflect.Value
	// made holds the pointers this cloner created, so a pointer reached twice is copied once.
	made map[cloneKey]struct{}
}

// cloneKey identifies a pointer, map or slice by its type, address and length.
type cloneKey struct {
	t   reflect.Type
	p   unsafe.Pointer
	len int
}

func (c *cloner) remember(key cloneKey, clone reflect.Value) {
	if c.seen == nil {
		c.seen = make(map[cloneKey]reflect.Value)
		c.made = make(map[cloneKey]struct{})
	}
	c.seen[key] = clone
	if clone.Kind() == reflect.Pointer {
		c.made[cloneKey{t: clone.Type(), p: clone.UnsafePointer()}] = struct{}{}
	}
}

// cloneValue deep-copies the settable value v in place, so mutating what the strip walk reaches cannot affect the source. Opaque
// structs (no exported fields, e.g. time.Time) stay the value copy. Interface-typed values are leaves: the walk never mutates through
// an interface (stripping one clears the field itself), so the pointee is safely shared. It returns [ErrMaxDepthExceeded] when
// nested structs exceed the budget [Strip] enforces.
func (c *cloner) cloneValue(v reflect.Value, depth int) error {
	switch v.Kind() {
	case reflect.Pointer:
		return c.clonePointer(v, depth, true)
	case reflect.Struct:
		return c.cloneStruct(v, depth)
	case reflect.Slice:
		return c.cloneSlice(v, depth)
	case reflect.Array:
		for i := range v.Len() {
			if err := c.cloneValue(v.Index(i), depth); err != nil {
				return err
			}
		}
	case reflect.Map:
		return c.cloneMap(v, depth)
	default:
		// Scalars and other leaves are already copied by value.
	}
	return nil
}

// clonePointer replaces the pointer v with a pointer to a copy of its target, descending into the copy when descend is set.
func (c *cloner) clonePointer(v reflect.Value, depth int, descend bool) error {
	if v.IsNil() {
		return nil
	}
	if _, ok := c.made[cloneKey{t: v.Type(), p: v.UnsafePointer()}]; ok {
		return nil
	}
	key := cloneKey{t: v.Type(), p: v.UnsafePointer()}
	if clone, ok := c.seen[key]; ok {
		v.Set(clone)
		return nil
	}
	nv := reflect.New(v.Type().Elem())
	c.remember(key, nv)
	nv.Elem().Set(v.Elem())
	v.Set(nv)
	if descend {
		return c.cloneValue(nv.Elem(), depth)
	}
	return nil
}

func (c *cloner) cloneStruct(v reflect.Value, depth int) error {
	if depth > c.o.maxDepth {
		return ErrMaxDepthExceeded
	}
	infos, err := fieldsFor(v.Type(), c.o.tagName)
	if err != nil {
		return err
	}
	for _, info := range infos {
		skip := info.kindMask&c.o.kindsMask != 0 || !info.recurse
		if skip && len(info.index) == 1 {
			continue
		}
		fv, ok := c.fieldFor(v, info.index)
		if !ok || skip {
			continue
		}
		if err := c.cloneValue(fv, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// fieldFor returns the settable field at index of v. Embedded pointers on the way, even of unexported types, are replaced by copies
// first, so a promoted field never aliases storage of the source. It reports false when a nil embedded pointer makes the field
// unreachable.
func (c *cloner) fieldFor(v reflect.Value, index []int) (reflect.Value, bool) {
	cur := v
	for i, x := range index {
		f := cur.Field(x)
		if !f.CanSet() && f.CanAddr() {
			f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		}
		if i < len(index)-1 && f.Kind() == reflect.Pointer {
			if f.IsNil() {
				return reflect.Value{}, false
			}
			if err := c.clonePointer(f, 0, false); err != nil {
				return reflect.Value{}, false
			}
			f = f.Elem()
		}
		cur = f
	}
	return cur, true
}

func (c *cloner) cloneSlice(v reflect.Value, depth int) error {
	if v.IsNil() {
		return nil
	}
	key := cloneKey{t: v.Type(), p: v.UnsafePointer(), len: v.Len()}
	if clone, ok := c.seen[key]; ok {
		v.Set(clone)
		return nil
	}
	nv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	c.remember(key, nv)
	reflect.Copy(nv, v)
	for i := range nv.Len() {
		if err := c.cloneValue(nv.Index(i), depth); err != nil {
			return err
		}
	}
	v.Set(nv)
	return nil
}

func (c *cloner) cloneMap(v reflect.Value, depth int) error {
	if v.IsNil() {
		return nil
	}
	key := cloneKey{t: v.Type(), p: v.UnsafePointer()}
	if clone, ok := c.seen[key]; ok {
		v.Set(clone)
		return nil
	}
	nv := reflect.MakeMapWithSize(v.Type(), v.Len())
	c.remember(key, nv)
	iter := v.MapRange()
	for iter.Next() {
		ev := reflect.New(v.Type().Elem()).Elem()
		ev.Set(iter.Value())
		if err := c.cloneValue(ev, depth); err != nil {
			return err
		}
		nv.SetMapIndex(iter.Key(), ev)
	}
	v.Set(nv)
	return nil
}
