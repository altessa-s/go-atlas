// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal_test

import (
	"testing"

	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
)

func BenchmarkLocal_Credentials(b *testing.B) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	for b.Loop() {
		_ = p.Credentials()
	}
}

func BenchmarkLocal_Name(b *testing.B) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	for b.Loop() {
		_ = p.Name()
	}
}
