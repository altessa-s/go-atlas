// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package optional provides a generic Optional[T] type that holds
// either a value of type T or nothing. It makes the "value may be
// absent" intent explicit at the type level, for places where Go's
// idiomatic (T, bool) pair or *T is awkward — struct fields, channel
// elements, slice values, map values.
//
// Internally Optional carries (value, present) by value. Some(v)
// does not allocate, the type is comparable when T is comparable,
// and the zero value of Optional[T] is a valid None. All exported
// constructors and methods are pure and safe for concurrent use.
//
// # Usage
//
//	// Struct field where nil-pointer would conflate "absent" with
//	// "present but zero".
//	type User struct {
//	    Name     string
//	    Nickname optional.Optional[string]
//	}
//
//	u := User{Name: "Ada", Nickname: optional.Some("")}
//	if nick, ok := u.Nickname.Get(); ok {
//	    fmt.Println("nickname is set, value:", nick) // distinguishes Some("") from None
//	}
//
//	// Bridge from the idiomatic (value, ok) tuple.
//	opt := optional.Of(m[k])
//
// # When NOT to use
//
// For ordinary lookups keep the (value, ok) idiom: `v, ok := m[k]`,
// `v, ok := <-ch`. Wrapping every such pair in Optional adds noise
// without removing any.
//
// For optional pointer fields where nil already unambiguously means
// "absent", *T is fine. Reach for Optional[T] when T's zero value is
// itself a valid Some and you need to distinguish it from None, or
// when you want the type signature to advertise optionality.
//
// For "value or error" use core/types/result.Result instead.
//
// # Serialization
//
// Optional implements bson.ValueMarshaler/bson.ValueUnmarshaler and
// json.Marshaler/json.Unmarshaler. Some(v) is encoded as the underlying
// value v; None is encoded as BSON null / JSON null. The IsZero method
// makes `bson:",omitempty"` strip None fields entirely on the wire.
//
// Some(zero(T)) is preserved through round-trip (it does not collapse
// to None), letting callers distinguish "absent" from "present but
// zero" — which is the main reason to reach for Optional in the first
// place. Standard encoding/json does not consult IsZero, so JSON
// Marshal of a None field always emits null; use *Optional[T] when JSON
// field omission matters.
//
// Because the BSON marshallers live on the type, this package depends
// on go.mongodb.org/mongo-driver/v2/bson — the only external dependency
// in core/*. The trade-off is intentional: making Optional usable as
// a first-class Mongo field type cannot be achieved from a sibling
// package.
package optional
