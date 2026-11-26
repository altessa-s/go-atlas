// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testdbname"
)

// Marker identifies database comments written by New, followed by a unique instance token.
const Marker = "created by go-atlas postgrestest"

type database struct {
	name   string
	oid    uint32
	marker string
}

type adminSession interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func createOwned(ctx context.Context, admin adminSession, name string) (database, error) {
	lock, err := tryNameLock(ctx, admin, name)
	if err != nil {
		return database{}, err
	}
	if !lock {
		return database{}, errors.New("database name already leased")
	}
	_, err = admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(name))
	if err != nil {
		return database{}, err
	}
	owned := database{name: name, marker: Marker + "; instance " + instanceToken()}
	if err = admin.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", name).Scan(&owned.oid); err != nil {
		return database{}, err
	}
	_, err = admin.Exec(ctx, "COMMENT ON DATABASE "+quoteIdent(name)+" IS '"+owned.marker+"'")
	if err != nil {
		return database{}, err
	}
	return owned, nil
}

func tryNameLock(ctx context.Context, admin adminSession, name string) (bool, error) {
	var ok bool
	err := admin.QueryRow(ctx, "SELECT pg_try_advisory_lock(736291,hashtext($1::text))", name).Scan(&ok)
	return ok, err
}

// dropOwned waits for closed clients to disappear from pg_stat_activity.
func dropOwned(ctx context.Context, admin adminSession, d database) error {
	for {
		err := dropChecked(ctx, admin, d)
		if !errors.Is(err, errBusy) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for target sessions and locks to clear: %w", ctx.Err())
		case <-time.After(teardownPollInterval):
		}
	}
}

// dropChecked returns only credential-safe errors.
// dropChecked is called on the session holding the name lock. The final DROP refuses new
// connections racing the catalog snapshot. A lost maintenance session cannot reconnect.
func dropChecked(ctx context.Context, admin adminSession, d database) error {
	const eligible = `SELECT EXISTS (
  SELECT FROM pg_database d WHERE d.datname=$1::text AND d.oid=$2
  AND shobj_description(d.oid,'pg_database')=$3
  AND EXISTS (SELECT FROM pg_locks l WHERE l.locktype='advisory' AND l.pid=pg_backend_pid()
    AND l.classid=736291 AND l.objid=(hashtext($1::text)::bigint & 4294967295)::oid AND l.objsubid=2 AND l.granted)),
  NOT EXISTS (SELECT FROM pg_stat_activity a WHERE a.datid=$2)
  AND NOT EXISTS (SELECT FROM pg_locks l WHERE l.database=$2 AND l.locktype='advisory')`
	var ok, idle bool
	if err := admin.QueryRow(ctx, eligible, d.name, d.oid, d.marker).Scan(&ok, &idle); err != nil {
		return dropDiagnostic("recheck database identity and activity", err)
	}
	if !ok {
		return errIneligible
	}
	if !idle {
		return errBusy
	}
	_, err := admin.Exec(ctx, "DROP DATABASE "+quoteIdent(d.name))
	return dropDiagnostic("execute DROP DATABASE", err)
}

func dropDiagnostic(step string, err error) error {
	if err == nil {
		return nil
	}
	states := sqldialect.SQLStates(err)
	if len(states) == 0 {
		return fmt.Errorf("%s failed (%T)", step, err)
	}
	return fmt.Errorf("%s failed (%T, SQLSTATE %s)", step, err, strings.Join(states, ", "))
}

var errBusy = errors.New("database still has sessions or locks")

var errIneligible = errors.New("database identity, ownership or lease changed")

func dropStale(ctx context.Context, admin *pgx.Conn, prefix string, cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, teardownTimeout)
	defer cancel()
	rows, err := admin.Query(ctx, `SELECT datname,oid,shobj_description(oid,'pg_database') FROM pg_database
 WHERE starts_with(datname,$1) AND shobj_description(oid,'pg_database') IS NOT NULL`, prefix)
	if err != nil {
		return err
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (database, error) {
		var d database
		scanErr := row.Scan(&d.name, &d.oid, &d.marker)
		return d, scanErr
	})
	if err != nil {
		return err
	}
	pattern := testdbname.Pattern(prefix)
	for _, d := range candidates {
		if !isMarker(d.marker) || !testdbname.Stale(pattern, d.name, cutoff) {
			continue
		}
		held, err := tryNameLock(ctx, admin, d.name)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		dropErr := dropChecked(ctx, admin, d)
		var released bool
		if err := admin.QueryRow(ctx, "SELECT pg_advisory_unlock(736291,hashtext($1::text))", d.name).Scan(&released); err != nil {
			return err
		}
		if !released {
			return errors.New("lost database name lock")
		}
		if dropErr != nil && !errors.Is(dropErr, errIneligible) && !errors.Is(dropErr, errBusy) {
			return dropErr
		}
	}
	return nil
}

// markerPattern is the whole comment New writes: [Marker], optionally followed by its instance token, and nothing else.
var markerPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(Marker) + `(; instance [0-9a-f]{16})?$`)

// isMarker reports whether comment is exactly a comment New writes: the bare [Marker] of earlier versions, or the marker with a
// 16-hex-digit instance token. A comment that merely starts with the marker does not qualify.
func isMarker(comment string) bool {
	return markerPattern.MatchString(comment)
}
