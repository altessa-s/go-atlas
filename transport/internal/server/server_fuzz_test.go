// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzBaseServer_Protocol(f *testing.F) {
	f.Add("http")
	f.Add("grpc")
	f.Add("")
	f.Add("custom-protocol")

	s := NewBaseServer(WithAddress(":0"))

	f.Fuzz(func(t *testing.T, protocol string) {
		result := s.Protocol(protocol)
		if result == "" {
			assert.Equal(t, "", protocol)
		}
	})
}
