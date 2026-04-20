// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package topics_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/broker/providers/nats/topics"
)

const (
	topicSingle   topics.Topic = "events.{TENANT}"
	topicTwo      topics.Topic = "orders.{REGION}.{ORDER_ID}"
	topicReports  topics.Topic = "reports.{TENANT}.{REPORT_ID}"
	topicQuotas   topics.Topic = "quotas.{TENANT}"
	topicNoMacros topics.Topic = "system.heartbeat"
	topicMidSubj  topics.Topic = "jobs.{KIND}.tail"
	topicAltUser  topics.Topic = "users.{USER_ID}"
)

const (
	keyTenant topics.TopicKey = "TENANT"
	keyRegion topics.TopicKey = "REGION"
	keyOrder  topics.TopicKey = "ORDER_ID"
	keyReport topics.TopicKey = "REPORT_ID"
	keyKind   topics.TopicKey = "KIND"
	keyUser   topics.TopicKey = "USER_ID"
)

func TestTopic_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic topics.Topic
		args  []string
		want  string
	}{
		{
			name:  "single macro",
			topic: topicSingle,
			args:  []string{keyTenant.String(), "acme"},
			want:  "events.acme",
		},
		{
			name:  "two macros in declared order",
			topic: topicTwo,
			args:  []string{keyRegion.String(), "us", keyOrder.String(), "42"},
			want:  "orders.us.42",
		},
		{
			name:  "two macros in reverse order",
			topic: topicTwo,
			args:  []string{keyOrder.String(), "99", keyRegion.String(), "eu"},
			want:  "orders.eu.99",
		},
		{
			name:  "no macros, no args",
			topic: topicNoMacros,
			args:  nil,
			want:  "system.heartbeat",
		},
		{
			name:  "sanitize dots in value",
			topic: topicSingle,
			args:  []string{keyTenant.String(), "acme.io"},
			want:  "events.acme_io",
		},
		{
			name:  "sanitize asterisks in value",
			topic: topicAltUser,
			args:  []string{keyUser.String(), "u*123"},
			want:  "users.u_123",
		},
		{
			name:  "sanitize greater-than in value",
			topic: topicQuotas,
			args:  []string{keyTenant.String(), "acme>prod"},
			want:  "quotas.acme_prod",
		},
		{
			name:  "sanitize multiple problematic characters",
			topic: topicReports,
			args:  []string{keyTenant.String(), "ac.me*x>1", keyReport.String(), "r.id*7"},
			want:  "reports.ac_me_x_1.r_id_7",
		},
		{
			name:  "preserve single wildcard",
			topic: topicSingle,
			args:  []string{keyTenant.String(), topics.Wildcard},
			want:  "events.*",
		},
		{
			name:  "preserve full wildcard at end",
			topic: topicAltUser,
			args:  []string{keyUser.String(), topics.FullWildcard},
			want:  "users.>",
		},
		{
			name:  "preserve wildcards in multiple macros",
			topic: topicTwo,
			args:  []string{keyRegion.String(), topics.Wildcard, keyOrder.String(), topics.Wildcard},
			want:  "orders.*.*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.topic.With(tt.args...)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTopic_With_Panics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		topic       topics.Topic
		args        []string
		panicSubstr string
	}{
		{
			name:        "missing arguments for macro topic",
			topic:       topicSingle,
			args:        nil,
			panicSubstr: "expected 1 key-value pairs, got 0 arguments",
		},
		{
			name:        "odd number of arguments",
			topic:       topicSingle,
			args:        []string{keyTenant.String()},
			panicSubstr: "expected 1 key-value pairs, got 1 arguments",
		},
		{
			name:        "wrong number of macro pairs",
			topic:       topicSingle,
			args:        []string{keyTenant.String(), "acme", "EXTRA", "value"},
			panicSubstr: "expected 1 key-value pairs, got 4 arguments",
		},
		{
			name:        "unknown macro",
			topic:       topicSingle,
			args:        []string{"UNKNOWN_MACRO", "value"},
			panicSubstr: "unknown macro 'UNKNOWN_MACRO'",
		},
		{
			name:        "missing required macro for two-macro topic",
			topic:       topicTwo,
			args:        []string{keyRegion.String(), "us"},
			panicSubstr: "expected 2 key-value pairs, got 2 arguments",
		},
		{
			name:        "arguments for topic without macros",
			topic:       topicNoMacros,
			args:        []string{"KEY", "value"},
			panicSubstr: "topic has no macros but 2 arguments provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				require.NotNil(t, r, "expected With to panic")
				msg, ok := r.(string)
				require.Truef(t, ok, "panic value should be string, got %T", r)
				require.Contains(t, msg, tt.panicSubstr)
			}()
			tt.topic.With(tt.args...)
		})
	}
}

func TestTopic_WithValidation_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic topics.Topic
		args  []string
		want  string
	}{
		{
			name:  "single macro",
			topic: topicSingle,
			args:  []string{keyTenant.String(), "acme"},
			want:  "events.acme",
		},
		{
			name:  "two macros",
			topic: topicTwo,
			args:  []string{keyRegion.String(), "us", keyOrder.String(), "42"},
			want:  "orders.us.42",
		},
		{
			name:  "wildcard value",
			topic: topicAltUser,
			args:  []string{keyUser.String(), topics.Wildcard},
			want:  "users.*",
		},
		{
			name:  "value sanitization",
			topic: topicSingle,
			args:  []string{keyTenant.String(), "ab.12*cd>ef"},
			want:  "events.ab_12_cd_ef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.topic.WithValidation(tt.args...)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTopic_WithValidation_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		topic      topics.Topic
		args       []string
		wantErr    error
		wantSubstr string
	}{
		{
			name:       "empty macro value",
			topic:      topicSingle,
			args:       []string{keyTenant.String(), ""},
			wantErr:    topics.ErrEmptyMacroValue,
			wantSubstr: "invalid value for macro 'TENANT'",
		},
		{
			name:       "invalid characters",
			topic:      topicSingle,
			args:       []string{keyTenant.String(), "test@value"},
			wantErr:    topics.ErrInvalidCharacters,
			wantSubstr: "invalid value for macro 'TENANT'",
		},
		{
			name:       "unicode characters rejected",
			topic:      topicSingle,
			args:       []string{keyTenant.String(), "тест"},
			wantErr:    topics.ErrInvalidCharacters,
			wantSubstr: "invalid value for macro 'TENANT'",
		},
		{
			name:       "unknown macro",
			topic:      topicSingle,
			args:       []string{"UNKNOWN", "value"},
			wantSubstr: "unknown macro 'UNKNOWN'",
		},
		{
			name:       "missing macro",
			topic:      topicTwo,
			args:       []string{keyRegion.String(), "us"},
			wantSubstr: "expected 2 key-value pairs, got 2 arguments",
		},
		{
			name:       "wrong argument count",
			topic:      topicSingle,
			args:       []string{keyTenant.String()},
			wantSubstr: "expected 1 key-value pairs, got 1 arguments",
		},
		{
			name:       "subject too long",
			topic:      topicSingle,
			args:       []string{keyTenant.String(), strings.Repeat("a", 250)},
			wantErr:    topics.ErrSubjectTooLong,
			wantSubstr: "exceeds maximum length",
		},
		{
			name:       "full wildcard not at end",
			topic:      topicMidSubj,
			args:       []string{keyKind.String(), topics.FullWildcard},
			wantErr:    topics.ErrInvalidWildcardPosition,
			wantSubstr: "full wildcard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.topic.WithValidation(tt.args...)
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			require.Contains(t, err.Error(), tt.wantSubstr)
		})
	}
}

func TestTopic_AcceptsMacros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic topics.Topic
		keys  []topics.TopicKey
		want  bool
	}{
		{
			name:  "single macro accepts correct key",
			topic: topicSingle,
			keys:  []topics.TopicKey{keyTenant},
			want:  true,
		},
		{
			name:  "single macro rejects wrong key",
			topic: topicSingle,
			keys:  []topics.TopicKey{keyRegion},
			want:  false,
		},
		{
			name:  "two macros accept all correct keys",
			topic: topicTwo,
			keys:  []topics.TopicKey{keyRegion, keyOrder},
			want:  true,
		},
		{
			name:  "two macros reject if one key missing",
			topic: topicTwo,
			keys:  []topics.TopicKey{keyRegion, keyTenant},
			want:  false,
		},
		{
			name:  "no keys, topic with macros",
			topic: topicSingle,
			keys:  nil,
			want:  true,
		},
		{
			name:  "no keys, topic without macros",
			topic: topicNoMacros,
			keys:  nil,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.topic.AcceptsMacros(tt.keys...))
		})
	}
}

func TestTopic_RequiredMacros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		topic topics.Topic
		want  []topics.TopicKey
	}{
		{
			name:  "single macro",
			topic: topicSingle,
			want:  []topics.TopicKey{keyTenant},
		},
		{
			name:  "two macros sorted alphabetically",
			topic: topicTwo,
			want:  []topics.TopicKey{keyOrder, keyRegion},
		},
		{
			name:  "reports topic sorted alphabetically",
			topic: topicReports,
			want:  []topics.TopicKey{keyReport, keyTenant},
		},
		{
			name:  "no macros",
			topic: topicNoMacros,
			want:  []topics.TopicKey{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.topic.RequiredMacros())
		})
	}
}

func TestTopic_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		topic      topics.Topic
		wantErr    bool
		wantSubstr string
	}{
		{name: "valid single macro", topic: topicSingle},
		{name: "valid two macros", topic: topicTwo},
		{name: "valid no macros", topic: topicNoMacros},
		{
			name:       "empty topic",
			topic:      topics.Topic(""),
			wantErr:    true,
			wantSubstr: "topic template cannot be empty",
		},
		{
			name:       "topic exceeds max length",
			topic:      topics.Topic(strings.Repeat("a", topics.MaxSubjectLength+1)),
			wantErr:    true,
			wantSubstr: "exceeds maximum length",
		},
		{
			name:       "unclosed macro",
			topic:      topics.Topic("test.{UNCLOSED"),
			wantErr:    true,
			wantSubstr: "unclosed macro starting at position",
		},
		{
			name:       "closing brace without opening",
			topic:      topics.Topic("test.INVALID}"),
			wantErr:    true,
			wantSubstr: "closing brace without opening brace",
		},
		{
			name:       "empty macro name",
			topic:      topics.Topic("test.{}"),
			wantErr:    true,
			wantSubstr: "empty macro name",
		},
		{
			name:       "invalid character in macro name",
			topic:      topics.Topic("test.{invalid-macro}"),
			wantErr:    true,
			wantSubstr: "invalid character",
		},
		{
			name:       "nested macros",
			topic:      topics.Topic("test.{OUTER{INNER}}"),
			wantErr:    true,
			wantSubstr: "nested or unclosed macro",
		},
		{
			name:       "macro not preceded by dot",
			topic:      topics.Topic("prefix{TENANT}"),
			wantErr:    true,
			wantSubstr: "must be preceded by '.'",
		},
		{
			name:       "macro not followed by dot",
			topic:      topics.Topic("{TENANT}suffix"),
			wantErr:    true,
			wantSubstr: "must be followed by '.'",
		},
		{
			name:       "macro glued between tokens",
			topic:      topics.Topic("foo.{X}bar"),
			wantErr:    true,
			wantSubstr: "must be followed by '.'",
		},
		{
			name:  "macro is entire template",
			topic: topics.Topic("{X}"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.topic.Validate()
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantSubstr)
		})
	}
}

func TestTopicKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key      topics.TopicKey
		wantStr  string
		wantTmpl string
	}{
		{keyTenant, "TENANT", "{TENANT}"},
		{keyRegion, "REGION", "{REGION}"},
		{keyOrder, "ORDER_ID", "{ORDER_ID}"},
		{keyReport, "REPORT_ID", "{REPORT_ID}"},
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.wantStr, tt.key.String())
			// template() is unexported; assert it indirectly via WithValidation: a
			// topic containing the key's template substitutes correctly.
			subj, err := topics.Topic("x."+tt.wantTmpl).WithValidation(tt.key.String(), "v")
			require.NoError(t, err)
			require.Equal(t, "x.v", subj)
		})
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()

	require.Equal(t, "*", topics.Wildcard)
	require.Equal(t, ">", topics.FullWildcard)
	require.Equal(t, 255, topics.MaxSubjectLength)
}
