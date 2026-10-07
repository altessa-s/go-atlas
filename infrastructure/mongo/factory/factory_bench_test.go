// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
)

func BenchmarkClientOptions(b *testing.B) {
	builder := New(&mongoconfig.Config{
		Hosts:       []string{"localhost:27017"},
		Database:    "testdb",
		MaxPoolSize: 100,
		RetryReads:  true,
	})
	b.ResetTimer()
	for b.Loop() {
		builder.ClientOptions()
	}
}

func BenchmarkBuildCredential_SCRAM(b *testing.B) {
	builder := New(&mongoconfig.Config{
		Credentials: &mongoconfig.Credentials{
			AuthMechanism: mongoconfig.AuthMechanismTypeSCRAMSHA256,
			Scram:         &mongoconfig.SCRAMCredentials{Username: "u", Password: "p", AuthSource: "admin"},
		},
	})
	b.ResetTimer()
	for b.Loop() {
		builder.buildCredential()
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := &mongoconfig.Config{
		Hosts:       []string{"localhost:27017"},
		Database:    "testdb",
		MaxPoolSize: 100,
	}
	for b.Loop() {
		New(cfg)
	}
}

func BenchmarkMongoBuilder_Build(b *testing.B) {
	cfg := &mongoconfig.Config{
		Hosts:       []string{"localhost:27017"},
		Database:    "testdb",
		MaxPoolSize: 100,
		RetryReads:  true,
	}
	builder := New(cfg)
	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		_, _ = builder.Build(ctx)
	}
}
