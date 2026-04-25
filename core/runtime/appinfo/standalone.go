// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// This file compiles only when the build tag is set to standalone.
// This file is used to define the configuration for the application when it
// is running in a standalone environment.
//
// Pass the build tag to the go command when building the application:
// go build -tags standalone

//go:build standalone

package appinfo

// IsStandalone reports whether the binary was built with the "standalone"
// build tag. When true, [EnvLabel] returns [StandaloneLabel].
var IsStandalone = true
