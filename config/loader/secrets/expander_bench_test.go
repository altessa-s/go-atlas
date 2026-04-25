// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/loader/secrets"
)

func BenchmarkHasSecrets_WithSecret(b *testing.B) {
	content := "password=$__secret{app:password}"
	for b.Loop() {
		_ = secrets.HasSecrets(content)
	}
}

func BenchmarkHasSecrets_NoSecret(b *testing.B) {
	content := "password=plaintext"
	for b.Loop() {
		_ = secrets.HasSecrets(content)
	}
}

func BenchmarkExpander_Expand(b *testing.B) {
	mgr := newMockManager(map[string]string{"app:password": "s3cret"})
	expander := secrets.New(mgr)
	ctx := b.Context()
	content := "pass=$__secret{app:password}"
	b.ResetTimer()
	for b.Loop() {
		_, _ = expander.Expand(ctx, content)
	}
}

func BenchmarkExpandString_NoSecrets(b *testing.B) {
	mgr := newMockManager(map[string]string{})
	ctx := b.Context()
	content := "plain text with no secrets"
	b.ResetTimer()
	for b.Loop() {
		_, _ = secrets.ExpandString(ctx, content, mgr)
	}
}
