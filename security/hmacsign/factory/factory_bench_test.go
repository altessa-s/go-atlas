// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/security/hmacsign/factory"

	webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
)

func BenchmarkVerifier(b *testing.B) {
	cfg := &webhookconfig.Signature{
		Scheme:            webhookconfig.SchemeStripe,
		Secret:            "whsec_bench",
		AdditionalSecrets: []redacted.RedactedString{"whsec_old"},
		Tolerance:         webhookconfig.DefaultTolerance,
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = factory.Verifier(cfg)
	}
}
