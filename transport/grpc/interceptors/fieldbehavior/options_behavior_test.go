// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import "testing"

func TestMigratedOptionBehavior(t *testing.T) {
	opts := defaultOptions()
	if opts.methodKinds != nil || opts.skipResponse {
		t.Fatal("method kinds and skipResponse must keep their zero defaults")
	}
	WithMethodKind("", KindCreate)(opts)
	if opts.methodKinds != nil {
		t.Fatal("empty method name must not allocate or change the map")
	}
	WithMethodKind("/svc/Unknown", Kind(99))(opts)
	WithMethodKind("/svc/Create", KindCreate)(opts)
	if len(opts.methodKinds) != 2 || opts.methodKinds["/svc/Unknown"] != Kind(99) {
		t.Fatal("WithMethodKind did not preserve entries or the supplied enum")
	}
	WithSkipResponse()(opts)
	if !opts.skipResponse || len(opts.methodKinds) != 2 {
		t.Fatal("WithSkipResponse changed unrelated state or failed to set its flag")
	}
}
