// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "strings"

// MethodOption configures a concrete auth method value (typically a pointer, e.g. *approle.AuthMethod).
// Keeping options as functions over the concrete value avoids generics pitfalls with pointer method sets.
type MethodOption[T any] func(T)

// Apply applies the provided options to v in order.
func Apply[T any](v T, opts ...MethodOption[T]) {
	for _, opt := range opts {
		opt(v)
	}
}

type mountPathSetter interface{ SetMountPath(path string) }

// WithMountPathOption sets a custom mount path for auth methods that embed BaseMethod.
func WithMountPathOption[T mountPathSetter, P interface{ string | *string }](path P) MethodOption[T] {
	return func(m T) {
		var p string
		switch t := any(path).(type) {
		case string:
			p = t
		case *string:
			if t == nil {
				return
			}
			p = *t
		}

		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		m.SetMountPath(strings.ToLower(p))
	}
}

// AppendMountPathOption appends a mount path option if mountPath is not empty.
// Useful for Vault SDK option slices like []approle.LoginOption / []userpass.LoginOption.
func AppendMountPathOption[T any](opts []T, mountPath string, with func(string) T) []T {
	if mountPath == "" {
		return opts
	}
	return append(opts, with(mountPath))
}
