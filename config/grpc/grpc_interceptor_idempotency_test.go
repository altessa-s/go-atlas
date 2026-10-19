// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
)

func TestIdempotencyInterceptor_Validate_KeySource(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate  func(*grpcconfig.IdempotencyInterceptor)
		wantErr bool
	}{
		"defaults":                 {mutate: func(*grpcconfig.IdempotencyInterceptor) {}},
		"no key field by default":  {mutate: func(c *grpcconfig.IdempotencyInterceptor) { require.Empty(t, c.IdempotencyKeyField) }},
		"request_id field":         {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.IdempotencyKeyField = "request_id" }},
		"empty format is uuidv4":   {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = "" }},
		"opaque":                   {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = middlewareconfig.IdempotencyKeyFormatOpaque }},
		"pattern":                  {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = "pattern"; c.KeyPattern = `req-\d+` }},
		"pattern without regexp":   {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = "pattern" }, wantErr: true},
		"pattern does not compile": {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = "pattern"; c.KeyPattern = "(" }, wantErr: true},
		"unknown format":           {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.KeyFormat = "ulid" }, wantErr: true},
		"negative length":          {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.MaxKeyLength = -1 }, wantErr: true},
		"verbs classifier": {mutate: func(c *grpcconfig.IdempotencyInterceptor) {
			c.MethodClassifier = grpcconfig.IdempotencyMethodClassifierVerbs
		}},
		"unknown classifier": {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.MethodClassifier = "all" }, wantErr: true},
		"mandatory lists": {mutate: func(c *grpcconfig.IdempotencyInterceptor) {
			c.MandatoryMethods = []string{"/a.B/Create"}
			c.MandatoryPatterns = []string{"^/a\\.B/"}
		}},
		"bad mandatory pattern": {mutate: func(c *grpcconfig.IdempotencyInterceptor) { c.MandatoryPatterns = []string{"["} }, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := grpcconfig.DefaultIdempotencyInterceptor()
			cfg.Enabled = true
			tc.mutate(&cfg)
			if tc.wantErr {
				require.Error(t, cfg.Validate())
				return
			}
			require.NoError(t, cfg.Validate())
		})
	}
}
