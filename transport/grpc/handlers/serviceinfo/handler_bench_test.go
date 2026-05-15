// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"

	"google.golang.org/protobuf/types/known/emptypb"
)

func BenchmarkHandler_Get(b *testing.B) {
	h := serviceinfo.New(serviceinfo.WithServiceID("node-a"))
	ctx := context.Background()
	req := &emptypb.Empty{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := h.Get(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandler_GetWithLeader(b *testing.B) {
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeader(func(_ context.Context) (bool, string) {
			return false, "node-b"
		}),
	)
	ctx := context.Background()
	req := &emptypb.Empty{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := h.Get(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
