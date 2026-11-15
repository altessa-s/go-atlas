// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"

	baseserver "github.com/altessa-s/go-atlas/internal/transport/server"
)

func TestWithBaseOptionsEmptyPreservesExistingValues(t *testing.T) {
	opts := &options{baseOpts: []baseserver.Option{nil}}
	WithBaseOptions()(opts)
	if len(opts.baseOpts) != 1 {
		t.Fatalf("WithBaseOptions() changed existing base options: %#v", opts.baseOpts)
	}
}

func TestWithBaseOptionsAppendsAndPreservesArgumentSlice(t *testing.T) {
	existing := baseserver.WithAddress(":1")
	input := []baseserver.Option{baseserver.WithAddress(":2")}
	option := WithBaseOptions(input...)
	input[0] = nil // the variadic slice is read when the option is applied
	opts := &options{baseOpts: []baseserver.Option{existing}}
	option(opts)
	if len(opts.baseOpts) != 2 || opts.baseOpts[0] == nil || opts.baseOpts[1] != nil {
		t.Fatalf("WithBaseOptions did not append the captured option: %#v", opts.baseOpts)
	}
	input[0] = existing
	if opts.baseOpts[1] != nil {
		t.Fatal("mutating input after applying changed stored options")
	}
}

func TestWithBaseOptsReplacesAndPreservesArgumentSlice(t *testing.T) {
	input := []baseserver.Option{baseserver.WithAddress(":2")}
	option := WithBaseOpts(input...)
	input[0] = nil
	opts := &options{baseOpts: []baseserver.Option{baseserver.WithAddress(":1")}}
	option(opts)
	if len(opts.baseOpts) != 1 || opts.baseOpts[0] != nil {
		t.Fatalf("WithBaseOpts did not replace with the input value at apply time: %#v", opts.baseOpts)
	}
	input[0] = baseserver.WithAddress(":3")
	if opts.baseOpts[0] == nil {
		t.Fatal("WithBaseOpts must retain the input slice after applying")
	}
	WithBaseOpts()(opts)
	if len(opts.baseOpts) != 1 || opts.baseOpts[0] == nil {
		t.Fatalf("WithBaseOpts() changed existing values: %#v", opts.baseOpts)
	}
}
