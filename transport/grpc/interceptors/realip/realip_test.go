// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

func TestServerInterceptor_Name(t *testing.T) {
	extractor, err := clientip.NewExtractor()
	if err != nil {
		t.Fatal(err)
	}
	i := ServerInterceptor(extractor)
	if i.Name() != "realip" {
		t.Fatalf("Name = %q", i.Name())
	}
}

func TestServerInterceptor_ReturnsInterceptors(t *testing.T) {
	extractor, err := clientip.NewExtractor()
	if err != nil {
		t.Fatal(err)
	}
	i := ServerInterceptor(extractor)
	if i.ServerUnaryInterceptor() == nil {
		t.Fatal("ServerUnaryInterceptor should not be nil")
	}
	if i.ServerStreamInterceptor() == nil {
		t.Fatal("ServerStreamInterceptor should not be nil")
	}
}

func TestExtractOperationConstant(t *testing.T) {
	if extractOperation != "extract" {
		t.Fatalf("extractOperation = %q", extractOperation)
	}
}
