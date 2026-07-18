// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"os"
	"path/filepath"
	"time"

	loadersecrets "github.com/altessa-s/go-atlas/config/loader/secrets"
)

// defaultSecretsTimeout is the default timeout for secrets expansion operations.
const defaultSecretsTimeout = 5 * time.Second

// DefaultStructTagName is the default struct tag name for configuration field mapping.
const DefaultStructTagName = "yaml"

// DefaultMaxConfigBytes caps how many bytes the loader will read from a
// single configuration file. Without a cap, a symlink pointing to
// `/dev/zero`, a multi-GB tmpfs file, or a malicious include target
// could OOM the loader before the YAML parser ever rejected the input.
//
// 16 MiB is generous compared to realistic config files (typical
// service configs are a few KB to a few hundred KB) but tight enough
// that the worst-case read is bounded to a memory size the loader
// process can safely handle even on small instances.
const DefaultMaxConfigBytes int64 = 16 * 1024 * 1024

const (
	defaultValueTagName = "default"
	envTagName          = "env"
)

type options struct {
	path                string
	envPrefix           string
	envSectionDelimiter string `optgen:"default=DefaultEnvSectionDelimiter"`
	structTag           string `optgen:"default=DefaultStructTagName"`
	maxConfigBytes      int64  `optgen:"default=DefaultMaxConfigBytes"`
	skipEnv             bool
	skipDefaults        bool
	strict              bool `optgen:"manual"`
	secretsManager      loadersecrets.Manager
}

// WithStrict enables strict mode for configuration loading.
// In strict mode, the loader will return errors instead of silently ignoring:
//   - Undefined environment variables referenced via $VAR or ${VAR}
//   - Unsupported field types that cannot be set from string values
//   - Field assignment failures due to type incompatibility
//   - References to fields that do not exist in the configuration struct
func WithStrict() Option {
	return func(o *options) {
		o.strict = true
	}
}

// WithPathOnEnvKey sets the config path from an environment variable.
// If the environment variable exists, its value is used as the path.
// Otherwise, the defaultPath is used.
func WithPathOnEnvKey(envKey, defaultPath string) Option {
	return func(opt *options) {
		if cfgFile, ok := os.LookupEnv(envKey); ok {
			WithPath(cfgFile)(opt)
			return
		}

		WithPath(defaultPath)(opt)
	}
}

func (o *options) normalization() {
	// Keep previous behavior: empty path means "unset". If set, clean it.
	if o.path != "" {
		o.path = filepath.Clean(o.path)
	}
}

// getSecretsContext returns a context with timeout for secrets operations.
// Creates a new context with defaultSecretsTimeout (5 seconds).
// The returned cancel function should be called when the operation completes.
func (o *options) getSecretsContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultSecretsTimeout)
}
