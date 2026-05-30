// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/loader"
)

func FuzzLoad_EnvParsing(f *testing.F) {
	f.Add("APP_NAME", "fuzz_app")
	f.Add("PORT", "not_an_int")
	f.Add("TAGS__0", "tag1")
	f.Add("DATABASE__HOST", "db_host")

	f.Fuzz(func(t *testing.T, key, value string) {
		if key == "" {
			return
		}

		t.Setenv(key, value)

		cfg := &TestConfig{}
		l := loader.New(nil)

		// We ignore errors, looking for panics or stability issues
		_, _ = l.Load(cfg)
	})
}
