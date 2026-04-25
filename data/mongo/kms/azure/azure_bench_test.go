// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsazure_test

import (
	"testing"

	kmsazure "github.com/altessa-s/go-atlas/data/mongo/kms/azure"
)

func BenchmarkAzure_Credentials(b *testing.B) {
	p := kmsazure.New("c", "s", "t", "k")
	for b.Loop() {
		_ = p.Credentials()
	}
}

func BenchmarkAzure_MasterKey(b *testing.B) {
	p := kmsazure.New("c", "s", "t", "k")
	for b.Loop() {
		_ = p.MasterKey()
	}
}
