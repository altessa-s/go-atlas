// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope

import (
	"iter"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// Registry maps an action key to the [Scope] required to perform it. The key is
// transport-neutral — a gRPC full method ("/pkg.Service/Method"), an HTTP route,
// or any other stable action identifier.
//
// A Registry is built once during initialization and read concurrently
// afterwards. Populate it with [Registry.Register] / [Registry.RegisterMany],
// then call [Registry.Freeze] before serving: after freezing it is an immutable
// snapshot safe for lock-free concurrent reads, and further registration panics.
//
// Lookups are deny-by-default: [Registry.Required] returns ok=false for an
// unregistered key, which [Enforcer.Enforce] treats as denied. Register a key
// with the empty scope to mark it intentionally public.
type Registry struct {
	// building is the mutable map used during the registration phase.
	building map[string]Scope
	// frozen is the immutable snapshot used after Freeze. Nil until frozen.
	frozen *coremaps.ImmutableMap[string, Scope]
}

// NewRegistry returns an empty [Registry] ready for registration.
func NewRegistry() *Registry {
	return &Registry{building: make(map[string]Scope)}
}

// Register records that key requires scope. The last registration for a key
// wins. Registering an empty scope marks key as intentionally public. Panics if
// called after [Registry.Freeze].
func (r *Registry) Register(key string, scope Scope) {
	if r.frozen != nil {
		panic("scope: Register called on frozen Registry")
	}
	r.building[key] = scope
}

// RegisterMany records that every key in keys requires scope. It is a
// convenience for bulk registration. Panics if called after [Registry.Freeze].
func (r *Registry) RegisterMany(scope Scope, keys ...string) {
	if r.frozen != nil {
		panic("scope: RegisterMany called on frozen Registry")
	}
	for _, key := range keys {
		r.building[key] = scope
	}
}

// Freeze converts the registry to an immutable snapshot safe for concurrent
// reads. Subsequent [Registry.Register] / [Registry.RegisterMany] calls panic.
// Freeze is idempotent.
func (r *Registry) Freeze() {
	if r.frozen != nil {
		return
	}
	r.frozen = coremaps.NewImmutableMap(r.building)
	r.building = nil
}

// Required returns the scope a key requires and whether the key is registered.
// Callers MUST honor ok: an unregistered key (ok=false) is denied by default. A
// registered public endpoint returns ("", true).
func (r *Registry) Required(key string) (Scope, bool) {
	if r.frozen != nil {
		return r.frozen.Get(key)
	}
	scope, ok := r.building[key]
	return scope, ok
}

// All returns an iterator over every registered key-to-scope mapping. It is
// safe for concurrent use after [Registry.Freeze]. Useful for auditing and
// generating permission documentation.
func (r *Registry) All() iter.Seq2[string, Scope] {
	if r.frozen != nil {
		return r.frozen.All()
	}
	return func(yield func(string, Scope) bool) {
		for k, v := range r.building {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Len returns the number of registered keys.
func (r *Registry) Len() int {
	if r.frozen != nil {
		return r.frozen.Len()
	}
	return len(r.building)
}
