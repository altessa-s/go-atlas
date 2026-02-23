// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import "testing"

func TestGetInterceptorName(t *testing.T) {
	tests := []struct {
		name string
		item any
		want string
	}{
		{"named", &NoOpInterceptor{}, "noop"},
		{"unnamed", "string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getInterceptorName(tt.item); got != tt.want {
				t.Fatalf("getInterceptorName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrderByDependencies_Empty(t *testing.T) {
	result, err := orderByDependencies(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestWrapUnwrapItems(t *testing.T) {
	items := []any{&NoOpInterceptor{}, &NoOpClientInterceptor{}}
	wrapped := wrapItems(items)
	if len(wrapped) != 2 {
		t.Fatalf("len = %d", len(wrapped))
	}
	if wrapped[0].Name() != "noop" {
		t.Fatalf("name = %q", wrapped[0].Name())
	}
	unwrapped := unwrapItems(wrapped)
	if len(unwrapped) != 2 {
		t.Fatalf("unwrapped len = %d", len(unwrapped))
	}
}
