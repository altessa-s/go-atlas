// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"

	serviceinfov1 "github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1"
)

func BenchmarkHandler_GetServiceInfo(b *testing.B) {
	h := serviceinfo.New(serviceinfo.WithServiceID("node-a"))
	ctx := context.Background()
	req := &serviceinfov1.GetServiceInfoRequest{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := h.GetServiceInfo(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandler_GetServiceInfoWithLeader(b *testing.B) {
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeader(func(_ context.Context) (bool, string) {
			return false, "node-b"
		}),
	)
	ctx := context.Background()
	req := &serviceinfov1.GetServiceInfoRequest{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := h.GetServiceInfo(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
