// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzParseTraceParent(f *testing.F) {
	f.Add("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	f.Add("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00")
	f.Add("")
	f.Add("invalid")
	f.Add("ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	f.Add("00-00000000000000000000000000000000-b7ad6b7169203331-01")

	f.Fuzz(func(t *testing.T, header string) {
		sc, ok := parseTraceParent(header)
		if ok {
			assert.NotEmpty(t, sc.traceID, "valid parse should have non-empty traceID")
			assert.NotEmpty(t, sc.spanID, "valid parse should have non-empty spanID")
		}
	})
}

func FuzzTraceContext_Extract(f *testing.F) {
	f.Add("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "")
	f.Add("", "")
	f.Add("invalid", "some-state")

	f.Fuzz(func(t *testing.T, traceparent, tracestate string) {
		tc := NewTraceContext()
		carrier := MapCarrier{}
		if traceparent != "" {
			carrier["traceparent"] = traceparent
		}
		if tracestate != "" {
			carrier["tracestate"] = tracestate
		}
		// Should not panic
		tc.Extract(t.Context(), carrier)
	})
}
