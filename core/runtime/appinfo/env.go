// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Well-known environment variable names used by [Env], [EnvOr], [EnvCached],
// and [HomeDir]. These constants avoid hard-coding variable names across callers.
const (
	// EnvHome is the standard Unix home-directory variable ("HOME").
	EnvHome = "HOME"

	// EnvUserProfile is the Windows user profile directory variable ("USERPROFILE").
	EnvUserProfile = "USERPROFILE"

	// EnvHomeDrive is the Windows home drive letter variable ("HOMEDRIVE"),
	// combined with [EnvHomePath] to form the home directory.
	EnvHomeDrive = "HOMEDRIVE"

	// EnvHomePath is the Windows home path variable ("HOMEPATH"),
	// combined with [EnvHomeDrive] to form the home directory.
	EnvHomePath = "HOMEPATH"

	// EnvNATSURL is the environment variable holding the NATS server URL.
	EnvNATSURL = "NATS_URL"

	// EnvAnthropicAPIKey is the environment variable holding the Anthropic API key.
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY" // #nosec G101 -- environment variable name, not a credential value

	// EnvVaultToken is the environment variable holding a HashiCorp Vault
	// authentication token.
	EnvVaultToken = "VAULT_TOKEN"
)

// envCache provides thread-safe caching of environment variable values.
// This is useful for frequently accessed environment variables.
var (
	envCacheMu sync.RWMutex
	envCache   = make(map[string]string)
)

// Env returns the value of the environment variable named by key,
// or an empty string if the variable is not set. It is a thin wrapper
// around [os.Getenv] provided for API consistency with [EnvOr] and [EnvCached].
func Env(key string) string {
	return os.Getenv(key)
}

// EnvOr returns the value of the environment variable named by key.
// If the variable is not set or is empty, defaultValue is returned instead.
func EnvOr(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// EnvCached returns the value of the environment variable named by key,
// caching the result on first access. Subsequent calls for the same key
// return the cached value without reading the environment again. This is
// useful for hot-path lookups of variables that do not change at runtime.
//
// EnvCached is safe for concurrent use. Call [ClearEnvCache] to invalidate
// all cached entries (e.g., in tests or after modifying the environment).
func EnvCached(key string) string {
	envCacheMu.RLock()
	if v, ok := envCache[key]; ok {
		envCacheMu.RUnlock()
		return v
	}
	envCacheMu.RUnlock()

	envCacheMu.Lock()
	defer envCacheMu.Unlock()

	// Double-check after acquiring write lock
	if v, ok := envCache[key]; ok {
		return v
	}

	v := os.Getenv(key)
	envCache[key] = v
	return v
}

// ClearEnvCache discards all entries stored by [EnvCached], forcing subsequent
// calls to re-read values from the environment. It is safe for concurrent use.
func ClearEnvCache() {
	envCacheMu.Lock()
	defer envCacheMu.Unlock()
	clear(envCache)
}

// HomeDir returns the current user's home directory by inspecting environment
// variables. On Windows it checks [EnvUserProfile] first, then the combination
// of [EnvHomeDrive] and [EnvHomePath]. On Unix it falls back to [EnvHome].
// An empty string is returned if none of the variables are set.
func HomeDir() string {
	// Check Windows environment variables first
	if home := os.Getenv(EnvUserProfile); home != "" {
		return home
	}
	if drive := os.Getenv(EnvHomeDrive); drive != "" {
		if path := os.Getenv(EnvHomePath); path != "" {
			return drive + path
		}
	}

	// Fall back to Unix/Linux home directory
	return os.Getenv(EnvHome)
}

// ExpandPath replaces a leading '~' in path with the value of [HomeDir].
// If path does not start with '~', it is returned unchanged.
func ExpandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		return filepath.Join(HomeDir(), path[1:])
	}
	return path
}

// GetEnvVar returns the value of the environment variable formed by prepending
// [EnvPrefix] (uppercased, with a "_" separator) to key. For example, if
// EnvPrefix is "MYAPP" and key is "config", the lookup is for "MYAPP_CONFIG".
// A leading '~' in the value is expanded to [HomeDir].
//
// For lookups that do not require the prefix, use [Env] or [EnvOr] instead.
func GetEnvVar(key string) string {
	if strings.ToUpper(EnvPrefix) != "" {
		key = strings.TrimRight(EnvPrefix, "_") + "_" + key
	}
	p := os.Getenv(strings.ToUpper(key))
	if len(p) > 0 && p[0] == '~' {
		p = filepath.Join(HomeDir(), p[1:])
	}
	return p
}
