// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"errors"
	"testing"
)

func BenchmarkRecordError(b *testing.B) {
	recorder := newBenchRecorder()
	failure := errors.New("failure")
	b.ReportAllocs()
	for b.Loop() {
		_, span := recorder.Start(b.Context(), "operation")
		span.RecordError(failure)
		span.End()
	}
}
