// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import "testing"

func TestConstants(t *testing.T) {
	if DefaultFallbackMessage == "" {
		t.Fatal("DefaultFallbackMessage is empty")
	}
	if ContentTypePlainText == "" {
		t.Fatal("ContentTypePlainText is empty")
	}
	if DefaultCodecJSON == "" {
		t.Fatal("DefaultCodecJSON is empty")
	}
}
