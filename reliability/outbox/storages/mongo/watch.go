// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/reliability/outbox/internal/watchutil"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Change-stream event fields and values used by the watch pipeline.
const (
	changeStreamFieldOperationType = "operationType"
	operationTypeInsert            = "insert"
)

// mongosMarker is the value MongoDB routers report in the hello reply's msg
// field. A router has no replica-set name of its own but does serve change
// streams, forwarding them to the shards.
const mongosMarker = "isdbgrid"

// Server error codes the watcher reacts to.
const (
	// commandNotFound marks a command the server does not know. It is how a
	// pre-4.4.2 server answers "hello", which only exists from that release on.
	commandNotFound = 59

	// changeStreamHistoryLost means the oplog has already rolled past the
	// resume token we offered, so the stream cannot be continued from it.
	changeStreamHistoryLost = 286

	// changeStreamNotSupported is the server's rejection of $changeStream on a
	// deployment that cannot serve it. [Store.SupportsChangeStreams] normally
	// catches this first; the code is checked as well because a deployment can
	// lose the capability between the probe and the open.
	changeStreamNotSupported = 40573
)

// Watch reconnect and shutdown timings. They are deliberately not options: the
// window they govern is bounded by the dispatch schedule either way, since the
// poll cycle keeps draining the outbox while the stream is down.
const (
	// watchRetryBaseDelay is the pause before the first reconnect attempt.
	watchRetryBaseDelay = watchutil.ReconnectBaseDelay

	// watchRetryMaxDelay caps the pause between reconnect attempts. It is well
	// under a typical dispatch interval, so a flapping stream costs latency
	// rather than throughput.
	watchRetryMaxDelay = watchutil.ReconnectMaxDelay

	// watchProbeTimeout bounds one attempt of the capability probe that
	// precedes the first stream, so a hung handshake is retried rather than
	// waited on forever.
	watchProbeTimeout = 10 * time.Second

	// watchMaxAwaitTime bounds how long the server holds an idle getMore before
	// answering with an empty batch. It sets how quickly a stopped watcher
	// notices cancellation, not how quickly an insert is reported.
	watchMaxAwaitTime = time.Second

	// watchCloseTimeout bounds the killCursors issued when a stream is torn
	// down. The caller's context is usually already canceled by then, and
	// closing on a dead context would leak the server-side cursor until it
	// times out on its own.
	watchCloseTimeout = 5 * time.Second

	// watchLogEvery spaces the warnings of a run of consecutive failures: the
	// first one is logged, then every watchLogEvery-th.
	watchLogEvery = watchutil.LogEvery
)

// watchPipeline restricts the change stream to the one transition the poll
// cycle cannot predict — a new event landing — and strips each change event
// down to its resume token.
//
// Inserts only, on purpose. A retry backoff elapsing and the unlock sweeper
// freeing a stuck lease are time-driven: they produce no insert, so no change
// stream could report them, and they remain the scheduled cycle's job.
//
// The projection is not a micro-optimization. An insert change event carries
// the full document by default, and an outbox payload may be close to a
// megabyte; the watcher reads nothing but _id, which is the resume token.
var watchPipeline = mongo.Pipeline{
	bson.D{{Key: "$match", Value: bson.M{changeStreamFieldOperationType: operationTypeInsert}}},
	bson.D{{Key: "$project", Value: bson.M{collectionFieldId: 1}}},
}

// helloReply is the subset of the hello/isMaster response that identifies the
// deployment topology.
type helloReply struct {
	SetName string `bson:"setName"`
	Msg     string `bson:"msg"`
}

// SupportsChangeStreams reports whether the deployment behind this store can
// serve change streams. They are built on the oplog, which only exists on a
// replica set (a single-node one counts) or behind a sharded cluster's router;
// a standalone mongod has none and rejects $changeStream outright.
//
// [Store.Watch] calls this before opening a stream, so callers only need it to
// decide up front whether to start a watcher at all.
func (s *Store) SupportsChangeStreams(ctx context.Context) (bool, error) {
	reply, err := s.hello(ctx)
	if err != nil {
		return false, err
	}
	return reply.SetName != "" || reply.Msg == mongosMarker, nil
}

// hello runs the topology handshake against the store's own database — the
// command is database-agnostic, so this avoids needing access to admin.
func (s *Store) hello(ctx context.Context) (helloReply, error) {
	db := s.collection.Database()

	var reply helloReply
	err := db.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&reply)
	if err == nil {
		return reply, nil
	}

	// "hello" landed in MongoDB 4.4.2; older servers answer only the legacy
	// spelling. Any other failure is real and is reported as such.
	srvErr, ok := errors.AsType[mongo.ServerError](err)
	if !ok || !srvErr.HasErrorCode(commandNotFound) {
		return helloReply{}, coreerrs.WrapOperation(err, "run MongoDB hello command")
	}

	if err = db.RunCommand(ctx, bson.D{{Key: "isMaster", Value: 1}}).Decode(&reply); err != nil {
		return helloReply{}, coreerrs.WrapOperation(err, "run MongoDB isMaster command")
	}
	return reply, nil
}

// Watch implements [outbox.Watcher] with a MongoDB change stream over the
// outbox collection, so a saved event wakes the dispatcher instead of waiting
// for its next poll tick. It blocks until ctx is done and returns nil on a
// clean shutdown.
//
// Returns [outbox.ErrWatchUnsupported] — without ever opening a stream — when
// the deployment cannot serve change streams; see [Store.SupportsChangeStreams].
// A probe that fails is not such an answer: it is retried with the reconnect
// backoff until the deployment answers or ctx ends.
//
// Inserts made inside a transaction are reported when that transaction commits,
// which is exactly the point at which the event became real: an aborted
// transaction produces no notification, and neither does an event whose
// business write rolled back.
//
// The driver resumes a broken stream on its own for transient failures. This
// loop covers what it cannot: a stream that ends for good is reopened from the
// last resume token after an exponential backoff, and a token the oplog has
// already discarded is dropped in favor of a fresh stream plus one synthetic
// wake-up, because the events missed in the gap are invisible to any stream
// that starts after them.
func (s *Store) Watch(ctx context.Context, notify func()) error {
	backoff := watchutil.ReconnectBackoff()

	supported, err := probeUntilAnswered(ctx, backoff, logProbe(s.logger, s.SupportsChangeStreams))
	if ctx.Err() != nil {
		return nil // Our own shutdown, before the deployment ever answered.
	}
	if err != nil {
		return err
	}
	if !supported {
		return outbox.ErrWatchUnsupported
	}

	var (
		resumeToken bson.Raw
		attempt     int
	)
	for {
		token, streamErr := s.streamInserts(ctx, resumeToken, notify)

		// A token that moved means the stream was alive and talking to us, so
		// whatever ended it counts as a first failure rather than the next in a
		// run — otherwise a watcher that reconnects once a day would still be
		// waiting the maximum delay months later.
		if progressed := len(token) > 0 && !bytes.Equal(token, resumeToken); progressed {
			attempt = 0
		}
		if len(token) > 0 {
			resumeToken = token
		}

		if ctx.Err() != nil {
			return nil // Our own shutdown; a torn-down stream is the expected outcome.
		}
		if isServerErrorCode(streamErr, changeStreamNotSupported) {
			// The deployment lost the capability under us (a replica set
			// reconfigured away, say). Retrying cannot fix that, so report it
			// the same way the up-front probe would have.
			return outbox.ErrWatchUnsupported
		}

		switch {
		case streamErr == nil:
			// A stream that ends without an error was invalidated: the
			// collection was dropped or renamed. Its token died with it.
			resumeToken = nil
		case isServerErrorCode(streamErr, changeStreamHistoryLost):
			// The oplog rolled past our token, so the gap is unrecoverable from
			// any stream. Start fresh from now and wake the dispatcher once —
			// events saved during the gap are still sitting in the collection.
			resumeToken = nil
			notify()
		}

		delay := backoff(attempt, streamErr)
		attempt++
		if watchutil.ShouldLogFailure(attempt) {
			s.logger.WarnContext(ctx, "outbox change stream ended, reconnecting",
				slog.Int("failures", attempt), slog.Duration("delay", delay), slog.Any("error", streamErr))
		}
		if !sleep(ctx, delay) {
			return nil
		}
	}
}

// probeUntilAnswered repeats the change-stream capability probe until the
// deployment answers it or ctx ends. A probe that fails says nothing about the
// capability — the primary may be stepping down, the network may have blinked —
// and giving up on it would leave the watcher off for the life of the process,
// so only an answer ends the wait. Each attempt is bounded by
// [watchProbeTimeout]; attempts are spaced by backoff.
func probeUntilAnswered(
	ctx context.Context, backoff coreretry.NextDelayFunc, probe func(context.Context) (bool, error),
) (supported bool, err error) {
	err = coreretry.Do(ctx, func(ctx context.Context) error {
		probeCtx, cancel := corectx.ApplyTimeout(ctx, watchProbeTimeout)
		defer cancel()

		var probeErr error
		supported, probeErr = probe(probeCtx)
		return probeErr
	},
		coreretry.WithMaxAttempts(-1),
		coreretry.WithNextDelay(backoff),
	)
	return supported, err
}

// logProbe wraps the capability probe so that a deployment that never answers
// — bad credentials, a wrong address — is visible: the failures of a run are
// warned about at the [watchLogEvery] rate, and an answer that follows failures
// is reported once. A probe cut short by the watcher's own shutdown is not a
// failure and is not logged.
func logProbe(logger *slog.Logger, probe func(context.Context) (bool, error)) func(context.Context) (bool, error) {
	failures := 0
	return func(ctx context.Context) (bool, error) {
		supported, err := probe(ctx)
		switch {
		case errors.Is(err, context.Canceled):
		case err != nil:
			failures++
			if watchutil.ShouldLogFailure(failures) {
				logger.WarnContext(ctx, "outbox change-stream probe failed, retrying",
					slog.Int("failures", failures), slog.Any("error", err))
			}
		case failures > 0:
			logger.InfoContext(ctx, "outbox change-stream probe answered after failures",
				slog.Int("failures", failures), slog.Bool("supported", supported))
		}
		return supported, err
	}
}

// streamInserts opens one change stream and pumps it until it ends, calling
// notify per change event. It returns the stream's final resume token — usable
// even when no event arrived, since the server advances it per batch — along
// with the error that ended the stream, if any.
func (s *Store) streamInserts(ctx context.Context, resumeToken bson.Raw, notify func()) (bson.Raw, error) {
	opts := mongoOptions.ChangeStream().SetMaxAwaitTime(watchMaxAwaitTime)
	if len(resumeToken) > 0 {
		opts.SetResumeAfter(resumeToken)
	}

	stream, err := s.collection.Watch(ctx, watchPipeline, opts)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "open outbox change stream")
	}
	defer func() {
		// Detached from ctx: by the time we unwind, ctx is usually the reason
		// the stream ended, and killCursors on a canceled context is a no-op
		// that leaves the cursor alive on the server.
		closeCtx, cancelClose := corectx.ApplyTimeout(context.WithoutCancel(ctx), watchCloseTimeout)
		defer cancelClose()
		_ = stream.Close(closeCtx)
	}()

	for stream.Next(ctx) {
		notify()
	}
	return stream.ResumeToken(), stream.Err()
}

// isServerErrorCode reports whether err carries the given MongoDB error code.
func isServerErrorCode(err error, code int) bool {
	if err == nil {
		return false
	}
	srvErr, ok := errors.AsType[mongo.ServerError](err)
	return ok && srvErr.HasErrorCode(code)
}

// sleep waits for d, reporting false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	return watchutil.Sleep(ctx, d)
}
