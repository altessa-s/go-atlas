// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsgcp_test

import (
	"testing"

	kmsgcp "github.com/altessa-s/go-atlas/data/mongo/kms/gcp"
)

func BenchmarkGoogle_Credentials(b *testing.B) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	for b.Loop() {
		_ = p.Credentials()
	}
}

func BenchmarkGoogle_MasterKey(b *testing.B) {
	p := kmsgcp.New("p", "e", "k", "l", "r", "n")
	for b.Loop() {
		_ = p.MasterKey()
	}
}
