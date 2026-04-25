// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package config loads and merges .optgen.yaml configuration files.
//
// [FindConfigFile] walks from a start directory toward the filesystem root,
// stopping at the first .optgen.yaml or at a project boundary (go.mod / .git).
// [Load] parses the YAML into a [Config], and [Config.GetPackageConfig] merges
// global [Defaults] with any [PackageConfig] overrides keyed by relative path.
//
// CLI flags always take precedence over config-file values.
//
// Example .optgen.yaml:
//
//	defaults:
//	  type: options
//	  formatter: "goimports -w"
//	packages:
//	  ./internal/auth:
//	    type: authOptions
//	    option-prefix: Auth
package config
