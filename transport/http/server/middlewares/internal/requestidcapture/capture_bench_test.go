// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestidcapture_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/internal/requestidcapture"
)

func BenchmarkRecord(b *testing.B) {
	ctx := requestidcapture.WithCapture(b.Context())
	for b.Loop() {
		requestidcapture.Record(ctx, "resolved")
	}
}
