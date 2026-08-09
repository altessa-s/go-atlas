// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/outbox"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	coretime "github.com/altessa-s/go-atlas/core/time"
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
	watchRetryBaseDelay = time.Second

	// watchRetryMaxDelay caps the pause between reconnect attempts. It is well
	// under a typical dispatch interval, so a flapping stream costs latency
	// rather than throughput.
	watchRetryMaxDelay = 30 * time.Second

	// watchRetryFactor is the exponential multiplier applied per attempt.
	watchRetryFactor = 1.5

	// watchRetryJitter spreads reconnects so that every instance of a service
	// that lost the same primary does not come back at the same instant.
	watchRetryJitter = 0.2

	// watchMaxAwaitTime bounds how long the server holds an idle getMore before
	// answering with an empty batch. It sets how quickly a stopped watcher
	// notices cancellation, not how quickly an insert is reported.
	watchMaxAwaitTime = time.Second

	// watchCloseTimeout bounds the killCursors issued when a stream is torn
	// down. The caller's context is usually already canceled by then, and
	// closing on a dead context would leak the server-side cursor until it
	// times out on its own.
	watchCloseTimeout = 5 * time.Second
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
	var srvErr mongo.ServerError
	if !errors.As(err, &srvErr) || !srvErr.HasErrorCode(commandNotFound) {
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
	supported, err := s.SupportsChangeStreams(ctx)
	if err != nil {
		return err
	}
	if !supported {
		return outbox.ErrWatchUnsupported
	}

	backoff := coreretry.Exponential(coreretry.ExponentialConfig{
		BaseDelay: watchRetryBaseDelay,
		MaxDelay:  watchRetryMaxDelay,
		Factor:    watchRetryFactor,
		Jitter:    watchRetryJitter,
	})

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
		if !sleep(ctx, delay) {
			return nil
		}
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
	var srvErr mongo.ServerError
	return errors.As(err, &srvErr) && srvErr.HasErrorCode(code)
}

// sleep waits for d, reporting false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(d)
	defer coretime.TimerStopAndDrain(timer)

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
