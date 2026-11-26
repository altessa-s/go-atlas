// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest

import (
	"cmp"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testdbname"
	"github.com/altessa-s/go-atlas/store/postgres"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

const (
	teardownTimeout       = 30 * time.Second
	heartbeatInterval     = time.Second
	heartbeatTimeout      = 5 * time.Second
	readinessPollInterval = 250 * time.Millisecond
	teardownPollInterval  = 25 * time.Millisecond
	maxIdentifier         = 63
)

var prefixPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// New opens a managed PostgreSQL pool in an isolated database and registers its cleanup.
// An empty DSN skips the test. The configured endpoint must be a direct test server.
func New(tb testing.TB, opts ...Option) *sql.DB {
	tb.Helper()
	o := newOptions(opts...)
	raw := os.Getenv(o.env)
	if raw == "" {
		tb.Skipf("%s not set; skipping integration test", o.env)
	}
	if !prefixPattern.MatchString(o.databasePrefix) || len(o.databasePrefix) > maxIdentifier-17 {
		tb.Fatalf("postgrestest: invalid database prefix")
	}
	ctx := tb.Context()
	dsn := normalizeDSN(raw)
	cfg, err := pgx.ParseConfig(dsn)
	check(tb, "parse DSN", err)
	ready, cancel := context.WithTimeout(ctx, o.readinessTimeout)
	defer cancel()
	// Verify the supplied endpoint before overriding its database. In particular a pooler may
	// expose only the application's database and reject attempts to open postgres.
	if database := cmp.Or(cfg.Database, cfg.User); database != "postgres" {
		check(tb, "verify direct configured endpoint", probeEndpoint(ready, cfg, database))
	}
	// Every cooperating caller takes name locks in the same maintenance database.
	cfg.Database = "postgres"
	admin, err := connectReady(ready, cfg)
	if err != nil {
		check(tb, "connect to maintenance database", err)
	}
	cleanup(tb, "close maintenance connection", admin.Close)
	check(tb, "verify direct maintenance connection", postgres.CheckDirect(ready, admin, "postgres"))
	cancel()
	var now time.Time
	check(tb, "read server clock", admin.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now))
	if o.staleAge > 0 {
		check(tb, "sweep stale databases", dropStale(ctx, admin, o.databasePrefix, now.Add(-o.staleAge)))
	}

	name := testdbname.New(o.databasePrefix, now)
	owned, err := createOwned(ctx, admin, name)
	check(tb, "create and mark "+name, err)
	// Only a confirmed COMMENT transfers ownership to the cleanup.
	boundedCleanup(tb, func(ctx context.Context) {
		if dropErr := dropOwned(ctx, admin, owned); dropErr != nil {
			tb.Errorf("postgrestest: drop database %s failed: %v", name, dropErr)
		}
	})

	cfg.Database = name
	lease, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		check(tb, "open target lease", err)
	}
	cleanup(tb, "close target lease", lease.Close)
	check(tb, "verify target lease connection", postgres.CheckDirect(ctx, lease, name))

	// The target session is independent of both the name lock and the working pool.
	_, err = lease.Exec(ctx, "SELECT pg_advisory_lock(736291)")
	check(tb, "acquire target lease", err)

	// The startup ping also borrows a database/sql connection internally.
	poolCtx, cancelPool := context.WithCancel(ctx)
	defer cancelPool()
	db, err := postgres.Open(poolCtx, redacted.RedactedString(withDatabase(dsn, name)))
	if err != nil {
		check(tb, "open working pool", err)
	}
	cleanup(tb, "close working pool", func(ctx context.Context) error { return closePool(ctx, db) })
	check(tb, "verify working database", checkWorking(ctx, db, name))
	watchLease(tb, admin, lease, db)
	return db
}

// Raw discards the physical connection if the driver panics during the identity check.
// This also lets the registered cleanup drop the database without evicting other sessions.
func checkWorking(ctx context.Context, db *sql.DB, name string) error {
	// Checkout can ping during ResetSession, after database/sql removes an idle
	// connection but before it returns a handle. Keep caller Done hooks out of
	// that window while preserving cancellation and the deadline. The identity
	// check uses the caller context once Raw owns the panic-safe release.
	checkout, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := db.Conn(checkout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return conn.Raw(func(raw any) error {
		c, ok := raw.(interface{ Conn() *pgx.Conn })
		if !ok {
			return errors.New("unexpected PostgreSQL connection type")
		}
		return postgres.CheckDirect(ctx, c.Conn(), name)
	})
}

func connectReady(ctx context.Context, cfg *pgx.ConnConfig) (*pgx.Conn, error) {
	var conn *pgx.Conn
	err := coreretry.Do(ctx, func(ctx context.Context) error {
		var connectErr error
		conn, connectErr = pgx.ConnectConfig(ctx, cfg)
		return connectErr
	}, coreretry.WithMaxAttempts(-1),
		coreretry.WithNextDelay(func(int, error) time.Duration { return readinessPollInterval }),
		coreretry.WithJoinLastError())
	return conn, err
}

func probeEndpoint(ctx context.Context, cfg *pgx.ConnConfig, database string) (err error) {
	c, err := connectReady(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), teardownTimeout)
		defer cancel()
		err = errors.Join(err, c.Close(closeCtx))
	}()
	return postgres.CheckDirect(ctx, c, database)
}

func check(tb testing.TB, op string, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("postgrestest: %v", sqldialect.SafeError(nil, op, err))
	}
}

func cleanup(tb testing.TB, op string, release func(context.Context) error) {
	tb.Helper()
	boundedCleanup(tb, func(ctx context.Context) {
		if err := release(ctx); err != nil {
			tb.Errorf("postgrestest: %v", sqldialect.SafeError(nil, op, err))
		}
	})
}

func boundedCleanup(tb testing.TB, release func(context.Context)) {
	tb.Helper()
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), teardownTimeout)
		defer cancel()
		release(ctx)
	})
}

// Stop the working pool if either dedicated session disappears. The watcher exits before
// cleanup uses the sessions again. A final probe covers tests shorter than one heartbeat.
func watchLease(tb testing.TB, admin, lease *pgx.Conn, db *sql.DB) {
	stop := make(chan struct{})
	done := make(chan struct{})
	tb.Cleanup(func() {
		close(stop)
		<-done
	})
	go func() {
		defer close(done)
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			final := false
			select {
			case <-stop:
				final = true
			case <-ticker.C:
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), heartbeatTimeout)
			err := admin.Ping(ctx)
			if err == nil {
				err = lease.Ping(ctx)
			}
			if err != nil {
				tb.Errorf("postgrestest: %v", sqldialect.SafeError(nil, "verify database lease", err))
				_ = closePool(ctx, db)
				cancel()
				return
			}
			cancel()
			if final {
				return
			}
		}
	}()
}

func closePool(ctx context.Context, db *sql.DB) error {
	done := make(chan error, 1)
	go func() { done <- db.Close() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func normalizeDSN(raw string) string {
	dsn := strings.TrimSpace(raw)
	scheme, rest, ok := strings.Cut(dsn, "://")
	if ok && (strings.EqualFold(scheme, "postgres") || strings.EqualFold(scheme, "postgresql")) {
		return strings.ToLower(scheme) + "://" + rest
	}
	return raw
}

// withDatabase returns dsn pointing at database name: the path of a URL DSN is replaced and its dbname and database parameters,
// which pgx applies after the path, are removed; a keyword/value DSN gets a dbname keyword, which overrides an earlier one. name is a
// plain identifier, so it needs no quoting in either form. [New] still checks the database it is connected to.
func withDatabase(dsn, name string) string {
	dsn = normalizeDSN(dsn)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		if u, err := url.Parse(dsn); err == nil {
			u.Path, u.RawPath = "/"+name, ""
			q := u.Query()
			q.Del("dbname")
			q.Del("database")
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return dsn + " dbname=" + name
}

// quoteIdent quotes a PostgreSQL identifier.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// instanceToken returns 16 random hex digits identifying one New call in its database's comment.
func instanceToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
