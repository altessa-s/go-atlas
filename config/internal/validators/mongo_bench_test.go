// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/validators"
)

func BenchmarkMongoDirectionConnectRule_Validate_SingleHost(b *testing.B) {
	hosts := []string{"mongodb://localhost:27017"}
	rule := validators.MongoDirectionConnect(hosts)
	value := true

	b.ResetTimer()
	for b.Loop() {
		_ = rule.Validate(value)
	}
}

func BenchmarkMongoDirectionConnectRule_Validate_MultipleHosts(b *testing.B) {
	hosts := []string{
		"mongodb://host1:27017",
		"mongodb://host2:27017",
		"mongodb://host3:27017",
	}
	rule := validators.MongoDirectionConnect(hosts)
	value := true

	b.ResetTimer()
	for b.Loop() {
		_ = rule.Validate(value)
	}
}

func BenchmarkMongoDirectionConnectRule_Validate_SRVHost(b *testing.B) {
	hosts := []string{"mongodb+srv://cluster.example.com"}
	rule := validators.MongoDirectionConnect(hosts)
	value := true

	b.ResetTimer()
	for b.Loop() {
		_ = rule.Validate(value)
	}
}

func BenchmarkMongoDirectionConnectRule_Validate_ConditionFalse(b *testing.B) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	rule := validators.MongoDirectionConnect(hosts).When(false)
	value := true

	b.ResetTimer()
	for b.Loop() {
		_ = rule.Validate(value)
	}
}

func BenchmarkMongoDirectionConnectRule_Validate_LargeHostList(b *testing.B) {
	hosts := make([]string, 100)
	for i := range 100 {
		hosts[i] = "mongodb://host" + string(rune(i)) + ":27017"
	}
	rule := validators.MongoDirectionConnect(hosts)
	value := true

	b.ResetTimer()
	for b.Loop() {
		_ = rule.Validate(value)
	}
}
