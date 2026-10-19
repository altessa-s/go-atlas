// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import "github.com/altessa-s/go-atlas/core/types/bits"

// Policy is the effective field-behavior policy of an [Engine]: the struct tag
// key it reads behavior kinds from and the kinds it strips. A translator that
// judges values the walk did not resolve — a nil pointer, an empty
// collection — from their declared type receives it through [PolicyBinder],
// so it applies the same policy as the resolved Strip marks. The zero Policy
// reads [DefaultTagName] and strips nothing, as an engine built without
// options. Policy is comparable, so it can key a cache.
type Policy struct {
	tagName string
	mask    uint8
}

// TagName returns the struct tag key the policy reads.
func (p Policy) TagName() string {
	if p.tagName == "" {
		return DefaultTagName
	}
	return p.tagName
}

// Strips reports whether the policy strips kind k.
func (p Policy) Strips(k Kind) bool {
	return k != Unspecified && bits.IsBitSet(p.mask, uint64(k))
}

// Kinds returns the stripped kinds in ascending order, nil when the policy
// strips nothing.
func (p Policy) Kinds() []Kind {
	var out []Kind
	for k := range Kind(len(kindTokens)) {
		if p.Strips(k) {
			out = append(out, k)
		}
	}
	return out
}

// PolicyBinder is implemented by a [Translator] that needs the engine's
// effective [Policy]. [New] calls BindPolicy once with the policy built from
// its options and drives the returned translator instead of the one passed
// in; the original is left unchanged, so one translator value may serve
// engines with different policies.
type PolicyBinder[Out any] interface {
	BindPolicy(p Policy) Translator[Out]
}

// policy returns the effective Policy of o. The default tag key is stored as
// the empty string, so equal policies compare equal and an engine built
// without options yields the zero Policy.
func (o *options) policy() Policy {
	p := Policy{tagName: o.tagName, mask: o.kindsMask}
	if p.tagName == DefaultTagName {
		p.tagName = ""
	}
	return p
}
