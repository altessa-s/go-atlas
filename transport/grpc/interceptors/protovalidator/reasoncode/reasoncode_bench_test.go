// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reasoncode_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"
)

func BenchmarkResolve(b *testing.B) {
	r := reasoncode.NewResolver(map[string]string{"acme.string.thing": "INVALID_THING"}, "acme.")
	for b.Loop() {
		_ = r.Resolve("int64.gte")
	}
}

func BenchmarkResolveViolation_Range(b *testing.B) {
	r := reasoncode.NewResolver(nil)
	v := benchViolation{ruleID: "int64.gte_lte", value: 0, hasValue: true, lower: 1, inclusive: true, hasLower: true}
	for b.Loop() {
		_ = r.ResolveViolation(v)
	}
}

func BenchmarkResolveViolation_Required(b *testing.B) {
	r := reasoncode.NewResolver(nil)
	v := benchViolation{ruleID: "required", fieldName: "userName"}
	for b.Loop() {
		_ = r.ResolveViolation(v)
	}
}

// benchViolation is a minimal [reasoncode.Violation] for benchmarks.
type benchViolation struct {
	ruleID    string
	fieldName string
	value     float64
	hasValue  bool
	lower     float64
	inclusive bool
	hasLower  bool
}

func (v benchViolation) RuleID() string                    { return v.ruleID }
func (v benchViolation) FieldName() string                 { return v.fieldName }
func (v benchViolation) NumericValue() (float64, bool)     { return v.value, v.hasValue }
func (v benchViolation) LowerBound() (float64, bool, bool) { return v.lower, v.inclusive, v.hasLower }
