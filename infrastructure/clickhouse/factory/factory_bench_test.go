// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
	chfactory "github.com/altessa-s/go-atlas/infrastructure/clickhouse/factory"
)

func BenchmarkClientOptionsFromFields(b *testing.B) {
	cfg := clickhouseconfig.Default()
	cfg.Hosts = []string{"ch-1:9000", "ch-2:9000"}
	cfg.Settings = map[string]string{"max_execution_time": "60"}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := chfactory.New(&cfg).ClientOptions(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientOptionsFromConnectionURI(b *testing.B) {
	cfg := clickhouseconfig.Default()
	cfg.Username = ""
	cfg.ConnectionURI = redacted.RedactedString("clickhouse://user:pass@host:9000/db")

	b.ReportAllocs()

	for b.Loop() {
		if _, err := chfactory.New(&cfg).ClientOptions(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConfigValidate(b *testing.B) {
	cfg := clickhouseconfig.Default()

	b.ReportAllocs()

	for b.Loop() {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
