// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"bytes"
	"log/slog"
	"runtime/debug"
	"testing"
)

func BenchmarkCheckDepInfo(b *testing.B) {
	hostDeps := make([]*debug.Module, 100)
	pluginDeps := make([]ModDep, 100)
	for i := range 100 {
		path := "example.com/dep" + string(rune('a'+i%26))
		hostDeps[i] = &debug.Module{Path: path, Version: "v1.0.0"}
		pluginDeps[i] = ModDep{Path: path, Version: "v1.0.0"}
	}
	// Introduce a few mismatches.
	pluginDeps[10].Version = "v2.0.0"
	pluginDeps[50].Version = "v2.0.0"

	hostBI := func() *debug.BuildInfo {
		return &debug.BuildInfo{GoVersion: "go1.25.0", Deps: hostDeps}
	}
	di := &DepInfo{GoVersion: "go1.25.0", Deps: pluginDeps}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	b.ResetTimer()
	for b.Loop() {
		checkDepInfo(logger, "bench-plugin", di, hostBI)
	}
}

func BenchmarkNewDepInfoFromBuild(b *testing.B) {
	for b.Loop() {
		_ = NewDepInfoFromBuild()
	}
}
