// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package builtin

// Import builtin subpackages for side-effects (init-based plugin registration).
// Note: fields package is imported separately in plugin_generator.go to avoid import cycle.
import (
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/check"
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/defaults"
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/guard"
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/postprocess"
	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/plugin/builtin/transform"
)
