// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"
	"time"
)

func TestApplyRecorderOptions(t *testing.T) {
	cfg := ApplyRecorderOptions(
		WithInstrumentationVersion("1.0.0"),
		WithSchemaURL("https://schema.example.com"),
	)
	if cfg.InstrumentationVersion() != "1.0.0" {
		t.Errorf("InstrumentationVersion = %q", cfg.InstrumentationVersion())
	}
	if cfg.SchemaURL() != "https://schema.example.com" {
		t.Errorf("SchemaURL = %q", cfg.SchemaURL())
	}
}

func TestApplySpanStartOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplySpanStartOptions(
		WithSpanKind(SpanKindServer),
		WithAttributes(String("key", "val")),
		WithStartTimestamp(now),
	)
	if cfg.Kind() != SpanKindServer {
		t.Errorf("Kind = %v", cfg.Kind())
	}
	if len(cfg.Attributes()) != 1 {
		t.Errorf("Attributes len = %d", len(cfg.Attributes()))
	}
	if !cfg.Timestamp().Equal(now) {
		t.Errorf("Timestamp = %v", cfg.Timestamp())
	}
}

func TestApplySpanEndOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplySpanEndOptions(WithEndTimestamp(now))
	if !cfg.Timestamp().Equal(now) {
		t.Errorf("Timestamp = %v", cfg.Timestamp())
	}
}

func TestApplyEventOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplyEventOptions(
		WithEventTimestamp(now),
		WithEventAttributes(String("k", "v")),
		WithStackTrace(true),
	)
	if !cfg.Timestamp().Equal(now) {
		t.Errorf("Timestamp = %v", cfg.Timestamp())
	}
	if len(cfg.Attributes()) != 1 {
		t.Errorf("Attributes len = %d", len(cfg.Attributes()))
	}
	if !cfg.StackTrace() {
		t.Error("StackTrace should be true")
	}
}

func TestApplySpanStartOptions_Links(t *testing.T) {
	sc := newSpanContextImpl(nil)
	link := Link{SpanContext: sc}
	cfg := ApplySpanStartOptions(WithLinks(link))
	if len(cfg.Links()) != 1 {
		t.Errorf("Links len = %d", len(cfg.Links()))
	}
}

func TestLinksPool(t *testing.T) {
	links := GetLinks()
	if links == nil {
		t.Fatal("GetLinks returned nil")
	}
	PutLinks(links)

	links2 := GetLinksWithCapacity(8)
	if links2 == nil {
		t.Fatal("GetLinksWithCapacity returned nil")
	}
	PutLinks(links2)
}
