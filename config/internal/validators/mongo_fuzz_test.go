// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/validators"
)

func FuzzMongoDirectionConnectRule_Validate(f *testing.F) {
	// Seed corpus with various host patterns
	f.Add("mongodb://localhost:27017")
	f.Add("mongodb+srv://cluster.example.com")
	f.Add("mongodb://host1:27017,host2:27017")
	f.Add("MONGODB+SRV://CLUSTER.EXAMPLE.COM")
	f.Add("")
	f.Add("invalid://host")
	f.Add("mongodb://")
	f.Add("srv://")
	f.Add("mongodb+srv://")

	f.Fuzz(func(t *testing.T, host string) {
		// Test with single host
		hosts := []string{host}
		rule := validators.MongoDirectionConnect(hosts)

		// Should not panic with bool value
		_ = rule.Validate(true)
		_ = rule.Validate(false)

		// Should not panic with nil value
		_ = rule.Validate((*bool)(nil))

		// Should not panic with non-bool value
		_ = rule.Validate("true")
		_ = rule.Validate(123)
		_ = rule.Validate(nil)

		// Test with multiple hosts containing fuzzed input
		multiHosts := []string{host, "mongodb://localhost:27017"}
		multiRule := validators.MongoDirectionConnect(multiHosts)
		_ = multiRule.Validate(true)

		// Test with When condition
		_ = rule.When(false).Validate(true)
		_ = rule.When(true).Validate(true)
	})
}
