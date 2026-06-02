// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"

	"google.golang.org/protobuf/proto"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// FuzzStripCreate feeds arbitrary serialized Resource payloads through every
// Strip entry point. Both mutation and dry-run code paths must not panic and
// must terminate, even on adversarial wire bytes that drive list/map/oneof
// reflection through edge cases.
func FuzzStripCreate(f *testing.F) {
	seeds := [][]byte{
		nil,
		{},
		mustMarshal(f, &testpb.Resource{}),
		mustMarshal(f, fullResource()),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		r := &testpb.Resource{}
		if err := proto.Unmarshal(raw, r); err != nil {
			t.Skip()
		}

		_ = fieldbehavior.StripCreate(r)
		_ = fieldbehavior.StripUpdate(r)
		_ = fieldbehavior.StripResponse(r)
		_ = fieldbehavior.StripCreate(r, fieldbehavior.WithStrict())
	})
}

func mustMarshal(f *testing.F, m proto.Message) []byte {
	f.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		f.Fatalf("marshal seed: %v", err)
	}
	return b
}
