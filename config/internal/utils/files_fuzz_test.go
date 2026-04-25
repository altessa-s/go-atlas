// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package utils_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/utils"
)

func FuzzFindFile(f *testing.F) {
	f.Add("")
	f.Add("/dev/null")
	f.Add("/nonexistent")
	f.Add("relative/path")
	f.Add("./file.txt")

	f.Fuzz(func(t *testing.T, path string) {
		_ = utils.FindFile(path)
	})
}
