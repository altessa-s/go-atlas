// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkClientOptionsFromConfig(b *testing.B) {
	f := New()
	cfg := &config.Mongodb{
		Hosts:       []string{"localhost:27017"},
		Database:    "testdb",
		MaxPoolSize: 100,
		RetryReads:  true,
	}
	b.ResetTimer()
	for b.Loop() {
		f.ClientOptionsFromConfig(cfg)
	}
}

func BenchmarkBuildCredential_SCRAM(b *testing.B) {
	f := New()
	creds := &config.MongodbCredentials{
		AuthMechanism: config.MongoAuthMechanismTypeSCRAMSHA256,
		Scram:         &config.MongoSCRAMCredentials{Username: "u", Password: "p", AuthSource: "admin"},
	}
	b.ResetTimer()
	for b.Loop() {
		f.buildCredential(creds)
	}
}

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}
