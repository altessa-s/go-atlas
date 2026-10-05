// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import "testing"

func BenchmarkValueClone(b *testing.B) {
	v := NewValue("key", "a-secret-payload", []byte(`"a-secret-payload"`), "v1")
	for b.Loop() {
		_ = v.clone()
	}
}

func BenchmarkValueClone_Map(b *testing.B) {
	v := NewValue("key", map[string]string{"user": "u", "password": "p", "host": "h"}, nil, "v1")
	for b.Loop() {
		_ = v.clone()
	}
}
