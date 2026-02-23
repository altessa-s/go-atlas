// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"os"
	"path"
	"strings"
)

// BinDir returns the directory where application binaries are expected to reside.
// If the <EnvPrefix>_BIN_DIR environment variable is set (via [GetEnvVar]),
// that value is used after cleaning trailing slashes. Otherwise it defaults
// to "/opt/bin".
func BinDir() string {
	if dir := GetEnvVar("BIN_DIR"); dir != "" {
		return path.Clean(strings.TrimRight(dir, "/"))
	}
	return "/opt/bin"
}

// VarDir returns the base directory for variable runtime data.
// If the <EnvPrefix>_VAR_DIR environment variable is set (via [GetEnvVar]),
// that value is used after cleaning trailing slashes. Otherwise it defaults
// to "/var".
func VarDir() string {
	if dir := GetEnvVar("VAR_DIR"); dir != "" {
		return path.Clean(strings.TrimRight(dir, "/"))
	}
	return "/var"
}

// EtcDir returns the directory for application configuration files.
// If the <EnvPrefix>_ETC_DIR environment variable is set (via [GetEnvVar]),
// that value is used. Otherwise the path is constructed as
// "/etc/<Project>/<Name>" (with [Project] and [Name] lowercased).
// If [Project] is empty, the project component is omitted.
func EtcDir() string {
	if dir := GetEnvVar("ETC_DIR"); dir != "" {
		return strings.TrimRight(dir, "/")
	}

	var pathComps = []string{"/etc"}
	if Project != "" {
		pathComps = append(pathComps, strings.ToLower(Project))
	}
	pathComps = append(pathComps, strings.ToLower(Name))

	return path.Join(pathComps...)
}

// LibDir returns the directory for application library and state files.
// If the <EnvPrefix>_LIB_DIR environment variable is set (via [GetEnvVar]),
// that value is used. Otherwise the path is constructed as
// "<VarDir>/lib/<Project>/<Name>" (with [Project] and [Name] lowercased).
// If [Project] is empty, the project component is omitted.
func LibDir() string {
	if dir := GetEnvVar("LIB_DIR"); dir != "" {
		return strings.TrimRight(dir, "/")
	}

	var pathComps = []string{VarDir(), "lib"}
	if Project != "" {
		pathComps = append(pathComps, strings.ToLower(Project))
	}
	pathComps = append(pathComps, strings.ToLower(Name))

	p := path.Join(pathComps...)
	return p
}

// CertsCacheDir returns the directory for cached TLS certificates,
// located at [LibDir]/certs.
func CertsCacheDir() string {
	return LibDir() + "/certs"
}

// MakeAllDirs creates [VarDir] and [LibDir] (and any necessary parents) using
// os.MkdirAll with permission 0777 (the process umask applies). It returns the
// first error encountered, if any.
// #nosec G301 -- os.ModePerm (0777) is intentional for app directories, umask applies
func MakeAllDirs() error {
	dirs := []string{VarDir(), LibDir()}
	for i := range len(dirs) {
		if err := os.MkdirAll(dirs[i], os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}
