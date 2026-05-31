// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/orderby"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkTranslate_Simple(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b, "create_time desc")
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}

func BenchmarkTranslate_Multi(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b,
		"create_time desc, organization_id asc, slug, address.city desc, updated_at")
	trans := mustTranslator(b)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}

// BenchmarkTranslate_Wildcard locks in the O(P) overhead of the
// wildcard-prefix scan in IsFieldAllowed for a typical allow-list of
// mixed exact entries and wildcards.
func BenchmarkTranslate_Wildcard(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b,
		"create_time desc, address.city asc, user.profile.name desc")
	trans := mustTranslator(b,
		orderby.WithAllowedFields(
			"create_time",
			"address.*",
			"user.*",
			"billing.*",
			"audit.*",
		),
	)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}

// BenchmarkTranslate_PrefixMapping locks in the O(P) overhead of the
// fieldPrefixMapping linear scan in ApplyFieldMapping.
func BenchmarkTranslate_PrefixMapping(b *testing.B) {
	ob := testhelpers.MustParseOrderBy(b,
		"address.city desc, user.profile.name asc, billing.account.id desc")
	trans := mustTranslator(b,
		orderby.WithFieldPrefixMapping(map[string]string{
			"address.":      "addr.",
			"user.profile.": "users.prof.",
			"user.":         "users.",
			"billing.":      "bill.",
			"audit.":        "log.",
		}),
	)

	for b.Loop() {
		_, _ = trans.Translate(ob)
	}
}
