// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Connection configuration constants
const (
	// MinRequiredMongoMajorVersion is the minimum required MongoDB major version
	MinRequiredMongoMajorVersion = 5
	// DefaultPingMaxRetries is the default number of retry attempts for ping operations
	DefaultPingMaxRetries = 3
	// DefaultPingBaseDelay is the default base delay between ping retry attempts
	DefaultPingBaseDelay = 100 * time.Millisecond
)

// Server info command constants
const (
	// BuildInfoCommand is the MongoDB command to retrieve server build information
	BuildInfoCommand = "buildInfo"
	// VersionFieldName is the field name for version string in buildInfo response
	VersionFieldName = "version"
	// VersionArrayFieldName is the field name for version array in buildInfo response
	VersionArrayFieldName = "versionArray"
	// MinVersionArrayLength is the minimum required length for version array parsing
	MinVersionArrayLength = 2
)

type connection struct {
	externalClient     *mongo.Client
	clientOptions      *mongoOptions.ClientOptions
	transactionOptions TransactionOptions
	logger             *slog.Logger
	database           string
	client             atomic.Pointer[mongo.Client]
	ownsClient         bool
	// connectMu makes concurrent callers wait for the entire startup sequence.
	connectMu sync.Mutex
	connected bool
	closing   atomic.Int32 // close calls waiting for or holding connectMu
	metrics   *mongoMetrics
}

func (m *connection) open(ctx context.Context) error {
	if m.closing.Load() != 0 {
		return mongo.ErrClientDisconnected
	}
	client := m.externalClient
	if client == nil {
		opts := m.clientOptions
		var err error
		client, err = mongo.Connect(opts)
		if err != nil {
			return err
		}

		m.ownsClient = true
		query.RegisterClientEncoding(client, opts)
	}
	m.client.Store(client)

	if m.closing.Load() != 0 {
		return mongo.ErrClientDisconnected
	}
	if err := m.ping(ctx); err != nil {
		return err
	}
	if m.closing.Load() != 0 {
		return mongo.ErrClientDisconnected
	}
	return nil
}

func (m *connection) ping(ctx context.Context) error {
	return coreretry.Do(ctx, func(ctx context.Context) error {
		pingCtx, pingCtxCancel := corecontext.WithMaxTimeout(ctx, DefaultPingTimeout)
		defer pingCtxCancel()

		if err := m.client.Load().Ping(pingCtx, readpref.Primary()); err != nil {
			if !IsTransientTransaction(err) {
				return coreerrs.Wrap(err, "ping failed")
			}
			return err
		}
		return nil
	},
		coreretry.WithMaxAttempts(DefaultPingMaxRetries-1), // 0-based: attempts 0..N-1 = N total calls
		coreretry.WithShouldRetry(IsTransientTransaction),
		coreretry.WithNextDelay(func(attempt int, _ error) time.Duration {
			// Linear backoff: (attempt+1) * base delay
			return time.Duration(attempt+1) * DefaultPingBaseDelay
		}),
		coreretry.WithOnRetry(func(attempt int, err error, nextDelay time.Duration) {
			m.metrics.pingRetries.Inc()

			if m.logger != nil {
				m.logger.Debug("ping failed, retrying",
					slog.Int("attempt", attempt+1),
					slog.Int64("delay_ms", nextDelay.Milliseconds()),
					slog.Any("error", err))
			}
		}),
	)
}

// getServerInfo retrieves MongoDB server version information using the buildInfo command.
// It populates the VersionString, VersionMajor, and VersionMinor fields of the Mongo instance.
// Returns an error if the command fails or if version information cannot be parsed.
func (m *Mongo) getServerInfo(ctx context.Context) error {
	buildInfo, err := m.conn.executeBuildInfoCommand(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "execute buildInfo command")
	}

	if err = m.parseVersionString(buildInfo); err != nil {
		if m.conn.logger != nil {
			m.conn.logger.Warn("failed to parse version string", "error", err)
		}
	}

	if err = m.parseVersionArray(buildInfo); err != nil {
		if m.conn.logger != nil {
			m.conn.logger.Warn("failed to parse version array", "error", err)
		}
	}

	// If we couldn't get version info at all, return error
	if m.VersionString == "" && m.VersionMajor == 0 {
		return errors.New("unable to determine MongoDB server version")
	}

	return nil
}

// executeBuildInfoCommand executes the MongoDB buildInfo command and returns the response.
func (m *connection) executeBuildInfoCommand(ctx context.Context) (bson.M, error) {
	var buildInfo bson.M

	cmd := bson.M{BuildInfoCommand: 1}
	opts := mongoOptions.RunCmd().SetReadPreference(readpref.Primary())

	err := m.client.Load().Database(m.database).RunCommand(ctx, cmd, opts).Decode(&buildInfo)
	if err != nil {
		return nil, err
	}

	return buildInfo, nil
}

// parseVersionString extracts and sets the version string from buildInfo response.
func (m *Mongo) parseVersionString(buildInfo bson.M) error {
	versionValue, exists := buildInfo[VersionFieldName]
	if !exists {
		return errors.New("version field not found in buildInfo response")
	}

	versionStr, ok := versionValue.(string)
	if !ok {
		return fmt.Errorf("version field is not a string, got %T", versionValue)
	}

	if versionStr == "" {
		return errors.New("version string is empty")
	}

	m.VersionString = versionStr
	return nil
}

// parseVersionArray extracts and sets major/minor version numbers from buildInfo response.
func (m *Mongo) parseVersionArray(buildInfo bson.M) error {
	versionArrayValue, exists := buildInfo[VersionArrayFieldName]
	if !exists {
		return errors.New("versionArray field not found in buildInfo response")
	}

	versionArray, ok := versionArrayValue.(bson.A)
	if !ok {
		return fmt.Errorf("versionArray field is not an array, got %T", versionArrayValue)
	}

	if len(versionArray) < MinVersionArrayLength {
		return fmt.Errorf("versionArray has insufficient elements: got %d, need at least %d",
			len(versionArray), MinVersionArrayLength)
	}

	major, ok := versionArray[0].(int32)
	if !ok {
		return fmt.Errorf("major version is not int32, got %T", versionArray[0])
	}

	minor, ok := versionArray[1].(int32)
	if !ok {
		return fmt.Errorf("minor version is not int32, got %T", versionArray[1])
	}

	m.VersionMajor = major
	m.VersionMinor = minor
	return nil
}

// Close gracefully closes all MongoDB connections and cleans up resources.
// It safely handles multiple calls and ensures the client is properly disconnected.
//
// The close process includes:
//   - Closing the encryption client (if configured)
//   - Disconnecting the main MongoDB client
//   - Logging the successful closure
//
// This method is safe to call multiple times and should be called when the
// MongoDB client is no longer needed, typically in a defer statement.
// Close waits for an in-flight Connect, including ping retries and migration,
// before starting its shutdown timeout.
func (m *Mongo) Close() {
	m.conn.closing.Add(1)
	m.conn.connectMu.Lock()
	defer m.conn.connectMu.Unlock()
	defer m.conn.closing.Add(-1)
	if m.conn.connected {
		m.closeLocked(context.Background())
	}
}

// closeLocked tears down a successful or partially established connection.
func (m *Mongo) closeLocked(ctx context.Context) {
	m.conn.connected = false
	if m.Client() == nil {
		return
	}
	if m.conn.logger != nil {
		defer m.conn.logger.Info("mongo connection closed")
	}
	ctx, cancel := corecontext.ApplyTimeout(context.WithoutCancel(ctx), DefaultCloseTimeout)
	defer cancel()
	if err := m.encryption.close(ctx); err != nil && m.conn.logger != nil {
		m.conn.logger.Warn("failed to close encryption client", "error", err)
	}
	if err := m.conn.disconnect(ctx); err != nil && m.conn.logger != nil {
		m.conn.logger.Warn("failed to disconnect mongo client", "error", err)
	}
}

func (m *connection) disconnect(ctx context.Context) error {
	if !m.ownsClient {
		return nil
	}
	client := m.client.Load()
	m.ownsClient = false
	query.ForgetClientEncoding(client)
	err := client.Disconnect(ctx)
	if errors.Is(err, mongo.ErrClientDisconnected) {
		return nil
	}
	return err
}
