// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package staplertest holds test doubles shared by the TLS provider tests: a
// [CountingStapler] that records OCSP staple requests and [Handshake], which
// drives one in-memory TLS handshake against a server config so the
// config's GetCertificate callback runs with a real handshake context.
//
//	stapler := &staplertest.CountingStapler{}
//	// ... build a provider with the stapler and take its TLS config ...
//	staplertest.Handshake(t, cfg)
//	require.Equal(t, int32(1), stapler.Calls())
package staplertest
