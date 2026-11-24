// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

// PostgresSecretDSN holds a diagnostic case and the spellings that must stay private.
type PostgresSecretDSN struct {
	Name, DSN string
	Secrets   []string
}

// PostgresSecretDSNs exercises pgx password spellings at parse and connect boundaries.
// Valid DSNs use an absent local socket and echo the decoded password as the database name,
// so returning the original connection error would disclose the secret even with pgx redaction.
func PostgresSecretDSNs() []PostgresSecretDSN {
	const decoded = "s3cret"
	return []PostgresSecretDSN{
		{"url query parameter", "postgres://u@localhost/s3cret?password=s3cret&host=%2Fabsent-pg-fixture&sslmode=disable", []string{decoded}},
		{"keyword value with quotes and escapes", `host=/absent-pg-fixture dbname='a\\bc' password='a\\bc'`, []string{`a\\bc`, `a\bc`}},
		{"parsed keyword value with blanks", "host=/absent-pg-fixture password = s3cret dbname = s3cret", []string{decoded}},
		{"keyword value with blanks around the equals sign", "host=localhost password = s3cret port=bad", []string{decoded}},
		{"percent-encoded url password", "postgres://u:s%33cr%65t@localhost/s3cret?host=%2Fabsent-pg-fixture&sslmode=disable", []string{"s%33cr%65t", decoded}},
	}
}
