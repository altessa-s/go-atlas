// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
)

func TestInterceptor_Name(t *testing.T) {
	i := Interceptor()
	mi := i.(metadataInterceptor) //nolint:errcheck
	if mi.Name() != "metadata" {
		t.Fatal("wrong name")
	}
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := Interceptor()
	mi := i.(metadataInterceptor) //nolint:errcheck
	if mi.Dependencies() != nil {
		t.Fatal("Dependencies should be nil")
	}
}

func TestInterceptor_DrivenInterceptor(t *testing.T) {
	i := Interceptor()
	d, ctx := i.DrivenInterceptor(t.Context())
	if ctx == nil {
		t.Fatal("context should not be nil")
	}

	// Should return NoopDriver
	resp, err := d.PreCall(ctx, nil)
	if resp != nil || err != nil {
		t.Fatal("NoopDriver should return nil, nil")
	}

	// Verify it implements the interface
	var _ driver.DrivenInterceptor = i
}
