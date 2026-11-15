// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"context"
	"testing"

	"github.com/altessa-s/go-aip/fieldmask/extract"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestMigratedOptionBehavior(t *testing.T) {
	opts := defaultOptions()
	if opts.methodKinds != nil || opts.updateExtractors != nil || opts.readExtractors != nil {
		t.Fatal("default option maps must remain nil")
	}
	if opts.skipReadMask || opts.strictMode {
		t.Fatal("migrated flags must default to false")
	}

	WithMethodKind("", KindRead)(opts)
	if opts.methodKinds != nil {
		t.Fatal("empty method name must not allocate or change the map")
	}
	WithMethodKind("/svc/Read", Kind(99))(opts)
	if opts.methodKinds["/svc/Read"] != Kind(99) {
		t.Fatal("WithMethodKind must retain unrecognized enum values")
	}
	WithSkipReadMask()(opts)
	WithStrictMode()(opts)
	if !opts.skipReadMask || !opts.strictMode {
		t.Fatal("flag options were not applied")
	}
	if len(opts.methodKinds) != 1 {
		t.Fatal("flag options changed methodKinds")
	}

	WithDefaultUpdateExtractor(nil)(opts)
	WithDefaultReadExtractor(nil)(opts)
	if opts.defaultUpdateExtractor != nil || opts.defaultReadExtractor != nil {
		t.Fatal("nil extractors must be ignored")
	}
	update := extract.UpdateFunc(func(context.Context, proto.Message) (*fieldmaskpb.FieldMask, proto.Message, func(*fieldmaskpb.FieldMask), bool) {
		return nil, nil, nil, false
	})
	read := extract.ReadFunc(func(context.Context, proto.Message) (*fieldmaskpb.FieldMask, bool) {
		return nil, false
	})
	WithDefaultUpdateExtractor(update)(opts)
	WithDefaultReadExtractor(read)(opts)
	if opts.defaultUpdateExtractor == nil || opts.defaultReadExtractor == nil {
		t.Fatal("non-nil extractors were not applied")
	}
}
