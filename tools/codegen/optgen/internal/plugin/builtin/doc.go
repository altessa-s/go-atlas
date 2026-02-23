// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package builtin provides shared helpers and template infrastructure used by
// the built-in optgen field-handler plugins.
//
// Concrete plugin implementations live in sub-packages that are registered via
// init-time side-effect imports (see import_subpackages.go):
//
//   - [check] -- validation checks (required, minlen, maxlen, oneof, nonzero)
//   - [defaults] -- type-specific default value providers (logger, map, serializer)
//   - [guard] -- early-return guards (notnil)
//   - [postprocess] -- post-assignment processors (dedup, nonempty, positive)
//   - [transform] -- value transformers (lower, upper, trimspaces, trimprefix, trimsuffix)
//
// The [fields] sub-package contains the field-handler plugins themselves (setter,
// appender, slice_set, bool_flag, manual, net_ip_parse) and is imported
// separately in the generator package to avoid an import cycle.
//
// This package exports types and functions that those sub-packages use to build
// option template data ([OptionData], [OptionBaseData]), execute templates
// ([ExecuteOptionTemplate], [MustOptionTemplate]), compose modifier pipeline
// phases ([BuildGuards], [BuildTransform], [BuildPostProcess], [BuildChecks]),
// and chain string transforms ([BuildStringTransformChain]).
package builtin
