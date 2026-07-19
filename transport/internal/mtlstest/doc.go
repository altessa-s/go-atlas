// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mtlstest provides a self-signed certificate authority for
// mutual-TLS integration tests across the transport layer. It is a thin
// wrapper over the shared test CA in internal/testhelpers that mints matching
// server and client leaf certificates (the latter carrying a SPIFFE URI SAN)
// signed by one root, so a real handshake can be driven from both ends without
// touching disk.
//
// It is shared by the gRPC and HTTP mTLS adapter integration tests, which both
// stand up a real RequireAndVerifyClientCert server and assert the peer
// certificate's identity reaches application code.
//
//	ca := mtlstest.NewCA(t)
//	serverCert := ca.ServerCert(t, net.IPv4(127, 0, 0, 1))
//	clientCert := ca.ClientCert(t, "spiffe://example.org/sa/billing")
//	// server: ClientCAs: ca.Pool, ClientAuth: tls.RequireAndVerifyClientCert
//	// client: RootCAs: ca.Pool
package mtlstest
