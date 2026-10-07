// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouseconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(c *clickhouseconfig.Config)
		wantErr bool
	}{
		{"defaults", func(*clickhouseconfig.Config) {}, false},
		{"no hosts", func(c *clickhouseconfig.Config) { c.Hosts = nil }, true},
		{"no database", func(c *clickhouseconfig.Config) { c.Database = "" }, true},
		{"zero max open conns", func(c *clickhouseconfig.Config) { c.MaxOpenConns = 0 }, true},
		{"negative max idle conns", func(c *clickhouseconfig.Config) { c.MaxIdleConns = -1 }, true},
		{"unknown compression", func(c *clickhouseconfig.Config) { c.Compression = "snappy" }, true},
		{"uri replaces hosts and database", func(c *clickhouseconfig.Config) {
			c.ConnectionURI = "clickhouse://host:9000/db"
			c.Hosts, c.Database = nil, ""
		}, false},
		{"uri with username", func(c *clickhouseconfig.Config) {
			c.ConnectionURI = "clickhouse://host:9000/db"
			c.Username = "u"
		}, true},
		{"uri with password", func(c *clickhouseconfig.Config) {
			c.ConnectionURI = "clickhouse://host:9000/db"
			c.Password = "p"
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := clickhouseconfig.Default()
			tc.mutate(&cfg)
			if tc.wantErr {
				require.Error(t, cfg.Validate())
				return
			}
			require.NoError(t, cfg.Validate())
		})
	}
}
