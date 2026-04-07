// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"context"
	"fmt"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	goplugin "plugin"
)

// symbolLookup mirrors the semantics of [plugin.Plugin.Lookup] but uses `any`
// instead of the named [plugin.Symbol] alias, so [resolveDescriptor] and
// [resolveInit] can be unit-tested with an in-memory stub that returns canned
// symbol values — no real .so file compilation required.
type symbolLookup func(name string) (any, error)

// symbolLookupFromPlugin adapts [*plugin.Plugin.Lookup] to the [symbolLookup]
// interface. The cast is necessary because [plugin.Symbol] is a named alias
// for `any` and Go's type system treats the two as distinct at the function
// signature level.
func symbolLookupFromPlugin(p *goplugin.Plugin) symbolLookup {
	return func(name string) (any, error) {
		return p.Lookup(name)
	}
}

// resolveDescriptor reads the "Descriptor" symbol from a plugin and unwraps
// it to a concrete [*Descriptor]. It accepts both supported declaration forms:
//
//	var Descriptor = plugins.Descriptor{Name: "x"}   // Lookup returns *Descriptor
//	var Descriptor = &plugins.Descriptor{Name: "x"}  // Lookup returns **Descriptor
//
// Go's [plugin.Lookup] returns a pointer to the package-level variable, so the
// first form yields `*Descriptor` and the second yields `**Descriptor`. The
// function-declaration form (`func Descriptor() plugins.Descriptor`) is not
// supported because it would force plugins to construct the descriptor
// lazily, which conflicts with the static metadata contract.
//
// Returns [ErrNoDescriptor] if the symbol is absent and [ErrInvalidDescriptor]
// when the symbol has an unsupported type or Name is empty.
func resolveDescriptor(lookup symbolLookup) (*Descriptor, error) {
	sym, err := lookup("Descriptor")
	if err != nil {
		return nil, ErrNoDescriptor
	}

	var desc *Descriptor
	switch v := sym.(type) {
	case *Descriptor:
		if v == nil {
			return nil, ErrInvalidDescriptor
		}
		desc = v
	case **Descriptor:
		if v == nil || *v == nil {
			return nil, ErrInvalidDescriptor
		}
		desc = *v
	default:
		return nil, coreerrs.Wrapf(ErrInvalidDescriptor, "got %T, want *Descriptor or **Descriptor", sym)
	}

	if desc.Name == "" {
		return nil, coreerrs.Wrap(ErrInvalidDescriptor, "Descriptor.Name is empty")
	}
	return desc, nil
}

// resolveInit reads the optional "Init" symbol from a plugin and unwraps it
// to a callable function. It accepts both supported declaration forms:
//
//	func Init(ctx context.Context) error { ... }       // Lookup returns func(...) error
//	var Init = func(ctx context.Context) error { ... } // Lookup returns *func(...) error
//
// The function declaration form is returned directly by [plugin.Lookup],
// while the variable form returns a pointer-to-function. Both are resolved
// here so plugin authors can use whichever style feels natural.
//
// Returns (nil, nil) when the symbol is absent — Init is optional. Returns
// a non-nil error when the symbol exists but has an unsupported type.
func resolveInit(lookup symbolLookup) (func(context.Context) error, error) {
	sym, err := lookup("Init")
	if err != nil {
		//nolint:nilnil // (nil, nil) is the "absent Init" sentinel; see doc.
		return nil, nil
	}

	switch fn := sym.(type) {
	case func(context.Context) error:
		return fn, nil
	case *func(context.Context) error:
		if fn == nil || *fn == nil {
			//nolint:nilnil // nil-valued variable is equivalent to absent.
			return nil, nil
		}
		return *fn, nil
	default:
		return nil, fmt.Errorf("plugin Init has unsupported type %T, want func(context.Context) error", sym)
	}
}
