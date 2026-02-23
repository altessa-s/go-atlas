// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmsaws_test

import (
	"testing"

	kmsaws "github.com/altessa-s/go-atlas/data/mongo/kms/aws"
)

func BenchmarkAmazon_Credentials(b *testing.B) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	for b.Loop() {
		_ = p.Credentials()
	}
}

func BenchmarkAmazon_MasterKey(b *testing.B) {
	p := kmsaws.New("AKID", "SECRET", "arn")
	for b.Loop() {
		_ = p.MasterKey()
	}
}
