// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	authgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

func BenchmarkAuthFunc(b *testing.B) {
	u, _ := url.Parse("spiffe://example.org/ns/default/sa/billing")
	cert := &x509.Certificate{URIs: []*url.URL{u}}
	ctx := peer.NewContext(b.Context(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}},
		},
	})
	authFn := mtls.AuthFunc()
	req := authgrpc.Request{}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := authFn(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
