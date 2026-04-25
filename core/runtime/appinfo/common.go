// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file compiles only when the build tag is set to non-standalone.
// This file is used to define the configuration for the application when it
// is running in a standalone environment.

//go:build !standalone

package appinfo

// IsStandalone reports whether the binary was built with the "standalone"
// build tag. When false (the default, for builds without the tag),
// [EnvLabel] returns [NonStandaloneLabel].
var IsStandalone = false
