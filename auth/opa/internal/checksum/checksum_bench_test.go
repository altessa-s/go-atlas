// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package checksum_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

func BenchmarkVerify(b *testing.B) {
	data := []byte("package test\ndefault allow = false")
	m := checksum.New(map[string]string{"test.rego": corehash.SHA256HexBytes(data)})
	errInvalid := errors.New("invalid")
	b.ReportAllocs()
	for b.Loop() {
		_ = m.Verify("test.rego", data, errInvalid, errInvalid)
	}
}
