// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestErrorTextRedactsBeforeTruncation(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("p", 2000)
	raw := "dial\r\n postgres://user%40example:" + secret + "@[::1]:5432/db or https://token@localhost/test"
	const safe = "dial postgres://[::1]:5432/db or https://localhost/test"
	require.Equal(t, safe, ErrorText(raw, DefaultMaxErrorLength))
	require.Equal(t, safe[:23]+"...", ErrorText(raw, 26))
}

func TestErrorTextRedactsCredentialsAcrossControlCharacters(t *testing.T) {
	t.Parallel()
	require.Equal(t, "dial https://db/atlas", ErrorText("dial https://user:pass\r\nword@db/atlas", DefaultMaxErrorLength))
}

func TestSanitizeBinaryError(t *testing.T) {
	t.Parallel()
	const limit = 20
	got := ErrorText("\n"+strings.Repeat("\xffx", limit), limit)
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len(got), limit)
}

func TestErrorTextRedactsAfterRemovingMalformedBytesAndControls(t *testing.T) {
	t.Parallel()
	const raw = "dial https:/\xff/worker:pa\r\nss@db/atlas"
	const safe = "dial https://db/atlas"
	require.Equal(t, safe, ErrorText(raw, len(safe)))
}

func TestErrorText_RedactsComplexURLsDSNsAndPasswords(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		raw       string
		forbidden []string
		want      string
	}{
		{
			name:      "password containing slash",
			raw:       "postgres://admin:s3cr/et@db/app",
			forbidden: []string{"admin", "s3cr", "et@"},
			want:      "postgres://db/app",
		},
		{
			name:      "slash in password after digits",
			raw:       "postgres://user:123/slash-secret@db/app",
			forbidden: []string{"user:", "slash-secret"},
			want:      "postgres://db/app",
		},
		{
			name:      "password containing question mark",
			raw:       "redis://u:p?w@h",
			forbidden: []string{"u:", "p?w"},
			want:      "redis://h",
		},
		{
			name:      "password containing fragment and at sign",
			raw:       "postgres://u:p#fr@g@db:5432/app",
			forbidden: []string{"u:", "p#fr", "fr@g"},
			want:      "postgres://db:5432/app",
		},
		{
			name:      "scheme-less mysql dsn",
			raw:       "user:secret@tcp(db:3306)/x",
			forbidden: []string{"user:", "secret"},
			want:      "tcp(db:3306)/x",
		},
		{
			name:      "quoted scheme-less dsn in error",
			raw:       `open "admin:s3cr/et@tcp(db:3306)/app": connection refused`,
			forbidden: []string{"admin:", "s3cr"},
			want:      `open "tcp(db:3306)/app": connection refused`,
		},
		{
			name:      "multiple credential fragments in one authority",
			raw:       "postgres://u:pwd=first-secret;password=second-secret@db/app",
			forbidden: []string{"first-secret", "second-secret"},
			want:      "postgres://db/app",
		},
		{
			name:      "postgres keyword password with punctuation and escaped space",
			raw:       `host=db password=one,two;marker\ suffix user=app`,
			forbidden: []string{"one", "two", "marker", "suffix"},
			want:      "host=db password=[REDACTED] user=app",
		},
		{
			name:      "postgres quoted password with escapes",
			raw:       `host=db password='literal\'secret,tail;marker' user=app`,
			forbidden: []string{"literal", "secret", "tail", "marker"},
			want:      "host=db password=[REDACTED] user=app",
		},
		{
			name: "at sign only in URL query",
			raw:  "https://api/x?email=a@b.com failed",
			want: "https://api/x?email=a@b.com failed",
		},
		{
			name: "at sign in query after credentials",
			raw:  "https://user:secret@api/x?email=a@b.com failed",
			want: "https://api/x?email=a@b.com failed",
		},

		{
			name:      "password fragment",
			raw:       "password=secret",
			forbidden: []string{"secret"},
		},
		{
			name:      "pwd fragment in semicolon dsn",
			raw:       "Server=db;Pwd=secret;Database=app",
			forbidden: []string{"secret"},
		},
		{
			name:      "password keyword with spaces and quotes",
			raw:       "host=db password = 's3cret value' port=5432",
			forbidden: []string{"s3cret"},
		},
		{
			name:      "url query password parameter",
			raw:       "postgres://u@localhost/app?password=s3cret&sslmode=disable",
			forbidden: []string{"u@", "s3cret"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ErrorText(tc.raw, DefaultMaxErrorLength)
			for _, secret := range tc.forbidden {
				require.NotContains(t, got, secret)
			}
			if tc.want != "" {
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func FuzzRedactURLUserinfo(f *testing.F) {
	const marker = "credential-marker-019"
	f.Add("initial")
	f.Add(`white space;comma,quote'"backslash\at@`)
	f.Add("postgres://u:pwd=one;password=two@db")
	f.Fuzz(func(t *testing.T, noise string) {
		RedactURLUserinfo(noise) // Arbitrary malformed text must not panic.
		suffix := url.QueryEscape(noise)
		for _, input := range []string{
			"postgres://u:pwd=" + marker + suffix + ";password=" + marker + "@db/app",
			`host=db password=initial,part;part\ ` + marker + suffix + " user=app",
			`host=db password='initial\'` + marker + suffix + "' user=app",
		} {
			require.NotContains(t, RedactURLUserinfo(input), marker)
		}
	})
}
