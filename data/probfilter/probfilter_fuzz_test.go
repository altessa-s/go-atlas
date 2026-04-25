// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

func FuzzManager_RegisterGet(f *testing.F) {
	f.Add("filter1")
	f.Add("")
	f.Add("special:name")

	f.Fuzz(func(t *testing.T, name string) {
		mgr := probfilter.NewManager()
		_, _ = mgr.Get(name)
	})
}
