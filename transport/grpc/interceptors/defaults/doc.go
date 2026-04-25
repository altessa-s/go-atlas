// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package defaults provides shared default values for gRPC interceptors.
// It centralizes common configuration to avoid duplication across interceptor packages.
//
// Example:
//
//	import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
//
//	var myIgnorePatterns = defaults.IgnorePatterns
package defaults
