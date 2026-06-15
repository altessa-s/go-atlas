// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

import (
	"strconv"
	"time"
)

// Tag is an HTTP entity-tag (RFC 7232 §2.3). It holds the inner opaque-tag
// value without the surrounding double quotes, plus a flag marking it weak.
//
// The zero Tag is the "absent" sentinel: [Tag.IsZero] reports true and it
// never matches another tag. Construct tags with [Strong], [Weak],
// [FromModTime], [Parse], or a [Generator]. Tag is comparable and safe to use
// as a map key.
type Tag struct {
	value string
	weak  bool
}

// Strong returns a strong entity-tag wrapping value (rendered as `"value"`).
// value must be a valid opaque-tag body: it may not contain a double quote or
// control characters. Generated tags always satisfy this; for untrusted input
// use [Parse] instead.
func Strong(value string) Tag {
	return Tag{value: value}
}

// Weak returns a weak entity-tag wrapping value (rendered as `W/"value"`).
// The same character constraints as [Strong] apply to value.
func Weak(value string) Tag {
	return Tag{value: value, weak: true}
}

// FromModTime returns a weak validator derived from an object's size and
// modification time, formatted as W/"<size16>-<unixnano16>" (both base-16).
// It performs no hashing, making it a cheap revalidation token in the style of
// nginx/Apache weak validators.
//
// It is weak by design: sub-second mtime resolution and clock skew mean equal
// tags only imply semantic, not byte-for-byte, equivalence — unsuitable for
// byte-range revalidation, where a strong content tag from a [Generator] is
// required.
func FromModTime(size int64, mod time.Time) Tag {
	buf := make([]byte, 0, 32)
	buf = strconv.AppendInt(buf, size, 16)
	buf = append(buf, '-')
	buf = strconv.AppendInt(buf, mod.UnixNano(), 16)
	return Tag{value: string(buf), weak: true}
}

// Value returns the inner opaque-tag value, without the surrounding quotes or
// any weak prefix.
func (t Tag) Value() string { return t.value }

// IsWeak reports whether t is a weak entity-tag.
func (t Tag) IsWeak() bool { return t.weak }

// IsZero reports whether t is the zero (absent) Tag.
func (t Tag) IsZero() bool { return t == Tag{} }

// String renders t as an HTTP header value: `"value"` when strong and
// `W/"value"` when weak. The zero Tag renders as `""`.
func (t Tag) String() string {
	if t.weak {
		return `W/"` + t.value + `"`
	}
	return `"` + t.value + `"`
}

// StrongMatch reports whether t and o match under the RFC 7232 §2.3.2 strong
// comparison: both tags must be strong and their opaque-tag values equal. Use
// it for If-Match and byte-range (If-Range) revalidation. The zero Tag never
// matches.
func (t Tag) StrongMatch(o Tag) bool {
	return !t.weak && !o.weak && t.value != "" && t.value == o.value
}

// WeakMatch reports whether t and o match under the RFC 7232 §2.3.2 weak
// comparison: their opaque-tag values are equal regardless of weakness. Use it
// for If-None-Match cache revalidation. The zero Tag never matches.
func (t Tag) WeakMatch(o Tag) bool {
	return t.value != "" && t.value == o.value
}
