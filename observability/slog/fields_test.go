// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"log/slog"
	"testing"
)

func TestFieldsToAttrs_GroupsAndOrder(t *testing.T) {
	fields := Fields{
		{Key: "user.id", Value: 42},
		{Key: "user.name", Value: "alice"},
		{Key: "request.id", Value: "req-1"},
		{Key: "plain", Value: true},
	}

	attrs := FieldsToAttrs(fields)
	if got, want := len(attrs), 3; got != want {
		t.Fatalf("attrs length = %d, want %d", got, want)
	}

	assertAttr(t, attrs[0], "plain", true)

	req := attrs[1]
	if req.Key != "request" || req.Value.Kind() != slog.KindGroup {
		t.Fatalf("request attr = %#v, want group with key request", req)
	}
	reqGroup := req.Value.Group()
	if len(reqGroup) != 1 || reqGroup[0].Key != "id" || reqGroup[0].Value.String() != "req-1" {
		t.Fatalf("request group = %#v, want single id=req-1", reqGroup)
	}

	user := attrs[2]
	if user.Key != "user" || user.Value.Kind() != slog.KindGroup {
		t.Fatalf("user attr = %#v, want group with key user", user)
	}
	userGroup := user.Value.Group()
	if len(userGroup) != 2 {
		t.Fatalf("user group len = %d, want 2", len(userGroup))
	}
	assertAttr(t, userGroup[0], "id", 42)
	assertAttr(t, userGroup[1], "name", "alice")
}

func TestFieldsToAttrs_DuplicateKeepsFirst(t *testing.T) {
	fields := Fields{
		{Key: "plain", Value: "first"},
		{Key: "plain", Value: "second"},
	}

	attrs := FieldsToAttrs(fields)
	if len(attrs) != 1 {
		t.Fatalf("attrs length = %d, want 1", len(attrs))
	}
	assertAttr(t, attrs[0], "plain", "first")
}

func assertAttr(t *testing.T, attr slog.Attr, wantKey string, wantVal any) {
	t.Helper()
	if attr.Key != wantKey {
		t.Fatalf("attr key = %s, want %s", attr.Key, wantKey)
	}

	switch v := wantVal.(type) {
	case bool:
		if attr.Value.Bool() != v {
			t.Fatalf("attr bool value = %v, want %v", attr.Value.Bool(), v)
		}
	case string:
		if attr.Value.String() != v {
			t.Fatalf("attr string value = %q, want %q", attr.Value.String(), v)
		}
	case int:
		if attr.Value.Int64() != int64(v) {
			t.Fatalf("attr int value = %d, want %d", attr.Value.Int64(), v)
		}
	default:
		if got := attr.Value.Any(); got != v {
			t.Fatalf("attr any value = %#v, want %#v", got, v)
		}
	}
}
