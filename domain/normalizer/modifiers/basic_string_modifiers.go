// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	stdStrings "strings"
)

func init() {
	registerBasicStringModifier("lowercase", corestrings.IsLowercaseUnsafe, stdStrings.ToLower)
	registerBasicStringModifier("uppercase", corestrings.IsUppercaseUnsafe, stdStrings.ToUpper)
	registerBasicStringModifier("trim", corestrings.IsTrimmed, stdStrings.TrimSpace)
}

func registerBasicStringModifier(name string, already func(string) bool, transform func(string) string) {
	RegisterModifier(name, func(v reflect.Value, _ map[string]string) ModifierResult {
		return ApplyStringModifier(v, func(str string) (string, bool) {
			if already != nil && already(str) {
				return str, false
			}
			out := transform(str)
			return out, out != str
		})
	})
}
