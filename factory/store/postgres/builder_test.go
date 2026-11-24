// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
	"github.com/altessa-s/go-atlas/store/postgres"

	postgresconfig "github.com/altessa-s/go-atlas/config/postgres"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuildValidation(t *testing.T) {
	t.Parallel()
	_, err := New(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
	cfg := postgresconfig.Default()
	b := New(&cfg).UseDefaultLogger().UseMeter(nil).UseTracer(nil).UseHealthCoordinator(nil)
	require.Same(t, slog.Default(), b.Logger())
	_, err = b.Build(t.Context())
	require.Error(t, err)
	cfg.DSN = "postgres://localhost/db"
	cfg.Pooling = "transaction"
	_, err = b.UseMigrator(func(context.Context, *sql.DB) error { t.Fatal("hook ran"); return nil }).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
	require.ErrorContains(t, err, "direct primary pool")
}

type healthRegistry map[string]health.Checker

func (r healthRegistry) RegisterService(name string, check health.Checker) { r[name] = check }

// poolEndpoint accepts the direct connector's startup, ping and transaction
// messages. No SQL evaluation is needed to check the pool configuration.
func poolEndpoint(t testing.TB, queryError func() *pgproto3.ErrorResponse) (redacted.RedactedString, <-chan map[string]string, <-chan string) {
	t.Helper()
	return poolEndpointTLS(t, queryError, nil)
}

func poolEndpointTLS(t testing.TB, queryError func() *pgproto3.ErrorResponse, server *tls.Config) (redacted.RedactedString, <-chan map[string]string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	startup := make(chan map[string]string, 8)
	statements := make(chan string, 32)
	var handlers sync.WaitGroup
	handlers.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			handlers.Go(func() {
				defer conn.Close()
				backend := pgproto3.NewBackend(conn, conn)
				m, err := backend.ReceiveStartupMessage()
				if err != nil {
					t.Error(err)
					return
				}
				if server != nil {
					if _, ok := m.(*pgproto3.SSLRequest); !ok {
						t.Error("expected TLS request")
						return
					}
					if _, err := conn.Write([]byte("S")); err != nil {
						t.Error(err)
						return
					}
					secure := tls.Server(conn, server)
					if err := secure.HandshakeContext(t.Context()); err != nil {
						return // tests also exercise failed certificate verification
					}
					backend = pgproto3.NewBackend(secure, secure)
					m, err = backend.ReceiveStartupMessage()
					if err != nil {
						t.Error(err)
						return
					}
				}
				startup <- m.(*pgproto3.StartupMessage).Parameters
				backend.Send(&pgproto3.AuthenticationOk{})
				backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
				if err := backend.Flush(); err != nil {
					t.Error(err)
					return
				}
				for {
					m, err := backend.Receive()
					if err != nil {
						if !errors.Is(err, io.EOF) {
							t.Error(err)
						}
						return
					}
					switch cmd := m.(type) {
					case *pgproto3.Terminate:
						return
					case *pgproto3.Query:
						statements <- cmd.String
						status := byte('I')
						if strings.HasPrefix(cmd.String, "BEGIN") {
							status = 'T'
						}
						var failure *pgproto3.ErrorResponse
						if queryError != nil {
							failure = queryError()
						}
						if failure != nil {
							backend.Send(failure)
						} else {
							backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("OK")})
						}
						backend.Send(&pgproto3.ReadyForQuery{TxStatus: status})
						if err := backend.Flush(); err != nil {
							t.Error(err)
							return
						}
					default:
						t.Errorf("unexpected frontend message %T", m)
						return
					}
				}
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); handlers.Wait() })
	dsn := "postgres://app@" + listener.Addr().String() + "/app"
	if server == nil {
		dsn += "?sslmode=disable"
	}
	return redacted.RedactedString(dsn), startup, statements
}

func TestBuildNamedPools(t *testing.T) {
	t.Parallel()
	primary, mainStartup, _ := poolEndpoint(t, nil)
	reserved, workerStartup, commands := poolEndpoint(t, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c := postgresconfig.Default()
	c.DSN = primary
	c.Pools = map[string]postgresconfig.Pool{"jobs": {DSN: reserved, MaxOpenConnections: 2, LockTimeout: time.Second}}
	checks := healthRegistry{}
	var migrated *sql.DB
	pools, err := New(&c).UseMigrator(func(_ context.Context, primary *sql.DB) error { migrated = primary; return nil }).UseHealthCoordinator(checks).Build(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pools.Close()) })
	require.Same(t, pools[postgres.DefaultPool], migrated)
	require.Equal(t, c.MaxOpenConnections, pools[postgres.DefaultPool].Stats().MaxOpenConnections)
	require.Equal(t, 2, pools["jobs"].Stats().MaxOpenConnections)
	require.Equal(t, "3000", (<-mainStartup)["lock_timeout"])
	require.Equal(t, "1000", (<-workerStartup)["lock_timeout"])
	require.Equal(t, health.StatusServing, checks["postgres.jobs"].CheckHealth(ctx))
	tx, err := postgres.BeginAsyncCommit(ctx, pools["jobs"])
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	var seen bool
	for len(commands) > 0 {
		if query := <-commands; strings.HasPrefix(query, "BEGIN") {
			require.Contains(t, query, "SET LOCAL lock_timeout = 1000")
			require.Contains(t, query, "SET LOCAL synchronous_commit = off")
			seen = true
		}
	}
	require.True(t, seen)
}

func TestMigrationDeadline(t *testing.T) {
	for _, tc := range []struct {
		name         string
		callerBudget time.Duration
	}{{"configured", 3 * time.Minute}, {"earlier caller", time.Minute}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := postgresconfig.Default()
			cfg.DSN, _, _ = poolEndpoint(t, nil)
			cfg.MigrationTimeout = 2 * time.Minute
			ctx, cancel := context.WithTimeout(t.Context(), tc.callerBudget)
			defer cancel()
			var deadline time.Time
			start := time.Now()
			pools, err := New(&cfg).UseMigrator(func(ctx context.Context, _ *sql.DB) error {
				var ok bool
				deadline, ok = ctx.Deadline()
				require.True(t, ok)
				return nil
			}).Build(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, pools.Close()) })
			if tc.callerBudget < cfg.MigrationTimeout {
				want, _ := ctx.Deadline()
				require.Equal(t, want, deadline)
			} else {
				require.WithinRange(t, deadline, start.Add(cfg.MigrationTimeout), time.Now().Add(cfg.MigrationTimeout))
			}
		})
	}
}

func TestMigrationFailureClosesPoolsAndPreservesCause(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		panics bool
	}{{"sentinel", false}, {"panic", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint, _, _ := poolEndpoint(t, nil)
			adapter := memory.New()
			meter := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("test"))
			cfg := postgresconfig.Default()
			cfg.DSN = endpoint
			cfg.Pools = map[string]postgresconfig.Pool{"worker": {MaxOpenConnections: 1}}
			checks := healthRegistry{}
			cause := errors.New("migration source sentinel")
			gauge := func(pool string) float64 {
				return adapter.GaugeValue("test_postgres_open_connections", map[string]string{"pool": pool})
			}
			hook := func(ctx context.Context, db *sql.DB) error {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 30*time.Second)
				require.NoError(t, db.PingContext(ctx))
				require.Equal(t, float64(1), gauge(postgres.DefaultPool), "the pool was open before migration")
				require.Equal(t, float64(1), gauge("worker"))
				if tc.panics {
					panic("migration panic")
				}
				return cause
			}
			build := func() error {
				_, err := New(&cfg).UseMeter(meter).UseHealthCoordinator(checks).UseMigrator(hook).Build(t.Context())
				return err
			}
			if tc.panics {
				require.PanicsWithValue(t, "migration panic", func() { _ = build() })
			} else {
				require.ErrorIs(t, build(), cause)
			}
			require.Empty(t, checks)
			require.Zero(t, gauge("worker"))
			require.Zero(t, gauge(postgres.DefaultPool), "pool and metric observer must close after migration failure")
		})
	}
}

func TestInjectedTLSMustVerifyServer(t *testing.T) {
	t.Parallel()
	cfg := postgresconfig.Default()
	cfg.DSN = "host=db"
	insecure := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // verify the factory rejects the injection
	_, err := New(&cfg).UseTlsConfig(insecure).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
	require.ErrorContains(t, err, "must verify the server")
}
