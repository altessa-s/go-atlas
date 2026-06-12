// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package appinfo provides standardized access to application metadata and configuration.
// Includes version, build info, environment configuration, and directory paths.
//
// All functions are read-only and thread-safe. Global variables are set during init
// and should not be modified at runtime.
//
// # Build Configuration
//
// Values can be set via ldflags during build:
//
//	go build -ldflags "
//	  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Name=MyApp'
//	  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Version=1.2.3'
//	  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.Commit=abc123'
//	  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'
//	  -X 'github.com/altessa-s/go-atlas/core/runtime/appinfo.EnvPrefix=MYAPP'
//	"
//
// # Usage
//
//	// Display version info
//	fmt.Println(appinfo.AppVersion())
//	fmt.Println(appinfo.Info()) // Compact: version=X.Y.Z, revision=ABC, env_prefix=PREFIX
//
//	// Directory paths
//	configPath := appinfo.EtcDir()           // /etc/<project>/<name>
//	dataPath := appinfo.LibDir()             // /var/lib/<project>/<name>
//
//	// Build information
//	fmt.Printf("Go version: %s\n", appinfo.BuildGoVersion())
//	fmt.Printf("Platform: %s\n", appinfo.BuildPlatform())
//
//	// Environment variables with prefix
//	// If EnvPrefix is MYAPP, this reads MYAPP_CONFIG
//	configFile := appinfo.GetEnvVar("CONFIG")
package appinfo
