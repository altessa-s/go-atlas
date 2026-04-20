// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package topics

// TopicKey identifies a macro placeholder declared in a [Topic] template.
// It is a named string type so callers can declare and pass macro keys without
// losing compile-time type information.
//
// TopicKey values are used with [Topic.AcceptsMacros] and
// [Topic.RequiredMacros]; the substitution methods [Topic.With] and
// [Topic.WithValidation] take the raw string form returned by [TopicKey.String].
type TopicKey string

// String returns the underlying macro name without curly braces, suitable as
// the key argument to [Topic.With] and [Topic.WithValidation].
func (k TopicKey) String() string {
	return string(k)
}

// template returns the macro placeholder syntax ("{NAME}") used inside topic
// templates. It is internal to the substitution logic.
func (k TopicKey) template() string {
	return "{" + string(k) + "}"
}
