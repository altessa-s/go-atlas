// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package naming_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/naming"
)

func TestOptionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"empty", "", ""},
		{"plain word", "logger", "Logger"},
		{"already capitalized", "Logger", "Logger"},
		{"camel case", "maxIdleTime", "MaxIdleTime"},

		{"bare initialism", "ttl", "TTL"},
		{"leading initialism", "httpClient", "HTTPClient"},
		{"trailing initialism", "entityId", "EntityID"},
		{"embedded initialism", "negativeTtl", "NegativeTTL"},
		{"two initialisms", "httpTlsConfig", "HTTPTLSConfig"},
		{"initialism before initialism-prefixed word", "idempotencyKeyEntityIdHeader", "IdempotencyKeyEntityIDHeader"},

		// A name that is already correct must survive unchanged, which is what
		// the all-caps-run boundary in splitWords buys.
		{"idempotent on correct name", "TTL", "TTL"},
		{"idempotent on correct compound", "HTTPClient", "HTTPClient"},
		{"idempotent on correct suffix", "EntityID", "EntityID"},

		// Not initialisms: a word that merely contains one must not be split.
		{"substring is not a word", "identity", "Identity"},
		{"substring at end", "valid", "Valid"},
		{"substring in middle", "shutdownTimeout", "ShutdownTimeout"},

		// gRPC is deliberately absent from the table — the repo spells it Grpc.
		{"grpc stays mixed caps", "grpcOptions", "GrpcOptions"},

		{"single letter", "n", "N"},
		{"digits", "utf8Encoding", "UTF8Encoding"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, naming.OptionName(tc.field))
		})
	}
}

// Applying the rule to its own output must be a no-op, otherwise regenerating
// an already-generated package would keep churning the option names.
func TestOptionName_Idempotent(t *testing.T) {
	t.Parallel()

	fields := []string{
		"ttl", "httpClient", "entityId", "uuidGenerator", "ipSubjectAlternativeNames",
		"xssProtectionDisabled", "maxIdleTime", "grpcOptions", "tlsConfig",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			once := naming.OptionName(field)
			require.Equal(t, once, naming.OptionName(once))
		})
	}
}
