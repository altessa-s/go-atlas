// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidIdentifier is returned when the table or cluster name is not a
// plain identifier.
var ErrInvalidIdentifier = errors.New("audit/clickhouse: invalid identifier")

// ErrInvalidEngine is returned when the engine is not a MergeTree-family
// engine with literal parameters.
var ErrInvalidEngine = errors.New("audit/clickhouse: invalid engine")

var (
	// tableNamePattern admits an unqualified identifier; the name is quoted
	// with backticks wherever it reaches SQL.
	tableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	// clusterNamePattern also admits the dots and hyphens cluster names in
	// remote_servers commonly carry.
	clusterNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

	// enginePattern admits a MergeTree-family engine name, optionally with
	// parameters that are each a single-quoted string without quotes or
	// backslashes (ZooKeeper paths, {shard}/{replica} macros) or a bare
	// identifier or number (a version column).
	enginePattern = regexp.MustCompile(`^[A-Za-z]*MergeTree` +
		`(\(\s*(` + engineArg + `(\s*,\s*` + engineArg + `)*)?\s*\))?$`)
)

const engineArg = `('[^'\\]*'|[A-Za-z0-9_]+)`

// validateSchemaNames checks the names [SchemaDDL] and the statements of the
// storage interpolate into SQL. They come from configuration, so they are
// held to a grammar that leaves no room for anything but a name.
func validateSchemaNames(table, engine, cluster string) error {
	if !tableNamePattern.MatchString(table) {
		return fmt.Errorf("%w: table %q", ErrInvalidIdentifier, table)
	}
	if cluster != "" && !clusterNamePattern.MatchString(cluster) {
		return fmt.Errorf("%w: cluster %q", ErrInvalidIdentifier, cluster)
	}
	if !enginePattern.MatchString(engine) {
		return fmt.Errorf("%w: %q", ErrInvalidEngine, engine)
	}
	return nil
}
