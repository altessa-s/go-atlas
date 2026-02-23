// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

// ErrInvalidArgument is the sentinel error wrapped by [InvalidArgument] when
// the caller-supplied condition is true. Use [errors.Is] to check for it.
var ErrInvalidArgument = errors.New("invalid argument")

// panicMsg is an internal helper function that panics with the given message.
// It supports both string values and pointers to strings.
func panicMsg[M interface{ ~string | ~*string }](msg M) {
	switch t := any(msg).(type) {
	case string:
		panic(t)
	case *string:
		panic(*t)
	default:
		panic(fmt.Sprintf("%#v", msg))
	}
}

// MustNonNil panics with msg if v is nil. Use it to enforce non-nil preconditions
// such as required constructor parameters or injected dependencies. The msg
// parameter accepts either a string or a *string.
func MustNonNil[M interface{ ~string | ~*string }](v any, msg M) {
	if v == nil {
		panicMsg(msg)
	}
}

// MustNonZero panics with msg if v is nil or the zero value for its type
// (as determined by [reflect.Value.IsZero]: false, 0, "", nil pointer, etc.).
// The msg parameter accepts either a string or a *string.
func MustNonZero[M interface{ ~string | ~*string }](v any, msg M) {
	if v == nil {
		panicMsg(msg)
		return
	}
	if reflect.ValueOf(v).IsZero() {
		panicMsg(msg)
	}
}

// MustError panics if err is non-nil. Use it to assert that an operation
// expected to always succeed does not fail; a non-nil error is treated as a
// programming error and causes a panic with the error value.
func MustError(err error) {
	if err != nil {
		panic(err)
	}
}

// MustResult returns val if err is nil, or panics with err otherwise.
// It is designed for wrapping (T, error) calls where failure indicates a
// programming error rather than a recoverable condition.
//
// Example:
//
//	s := panics.MustResult(NewService())
//	config := panics.MustResult(LoadConfig(path))
func MustResult[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// Must panics with msg if ok is false. It provides a concise way to enforce
// runtime invariants that should never be violated. The msg parameter accepts
// either a string or a *string.
//
// Example:
//
//	Must(len(items) > 0, "items cannot be empty")
//	Must(user != nil, "user is required")
func Must[M interface{ ~string | ~*string }](ok bool, msg M) {
	if !ok {
		panicMsg(msg)
	}
}

// InvalidArgument panics when ok is true, wrapping [ErrInvalidArgument] with
// the caller-supplied msg. This provides a standardized way to assert argument
// validity across an application. The panic value is an error that satisfies
// errors.Is(err, ErrInvalidArgument).
func InvalidArgument(ok bool, msg string) {
	if ok {
		panic(fmt.Errorf("%w: %s", ErrInvalidArgument, msg))
	}
}

// reallyPanic is used to determine whether to really panic.
var reallyPanic atomic.Bool

// PanicHandler is a callback invoked by [Handle] and [HandleWithOpts] after
// recovering from a panic. It receives the context of the panicking goroutine
// and the recovered value. Handlers must not panic themselves.
type PanicHandler func(ctx context.Context, r any)

// PanicHandlers is a convenience alias for a slice of [PanicHandler] functions,
// used when registering or replacing global handlers via [SetGlobalPanicHandlers].
type PanicHandlers = []PanicHandler

// LoggerFromContextFunc extracts an [slog.Logger] from a context. It should
// return nil when no logger is present. Register one via [SetLoggerFromContext]
// to allow the default panic handler to use application-specific context loggers
// without coupling to a particular logging library's context utilities.
type LoggerFromContextFunc func(ctx context.Context) *slog.Logger

// HandleOpts configures per-call behavior of [HandleWithOpts].
type HandleOpts struct {
	// ReallyPanic overrides the global re-panic setting (see [SetReallyPanic])
	// for a single HandleWithOpts call. When nil, the global setting applies.
	// When non-nil, its boolean value determines whether the panic is re-thrown
	// after all handlers have executed.
	ReallyPanic *bool
}

// NewHandleOpts creates a [HandleOpts] with all fields at their zero values
// (i.e., the global settings apply). Use method chaining to override:
//
//	opts := panics.NewHandleOpts().SetReallyPanic(false)
func NewHandleOpts() *HandleOpts {
	return &HandleOpts{}
}

// SetReallyPanic sets the per-call re-panic override and returns ho for
// method chaining. When rp is true, [HandleWithOpts] re-panics after
// executing all handlers; when false, the panic is swallowed.
func (ho *HandleOpts) SetReallyPanic(rp bool) *HandleOpts {
	ho.ReallyPanic = &rp
	return ho
}

// HandleWithOpts recovers from a panic and executes global handlers (registered
// via [AddGlobalPanicHandler] or [SetGlobalPanicHandlers]) followed by any
// handlers passed directly. Before invoking handlers, it runs
// [runtime.RunShutdownHooks] to flush buffered logs and telemetry.
//
// The opts parameter overrides global behavior (see [HandleOpts]). Pass nil
// to use the global defaults.
//
// HandleWithOpts must be called via defer at the top of the function:
//
//	func SomeFunction(ctx context.Context) {
//	    opts := panics.NewHandleOpts().SetReallyPanic(false)
//	    defer panics.HandleWithOpts(ctx, opts, customHandler)
//	    // ... code that might panic
//	}
func HandleWithOpts(ctx context.Context, opts *HandleOpts, handler ...PanicHandler) {
	handleWithContext(ctx, opts, handler...)
}

// Handle recovers from a panic and executes all registered global handlers
// followed by any additional handlers provided as arguments. If the global
// re-panic setting is true (see [SetReallyPanic]), the recovered value is
// re-panicked after all handlers complete.
//
// Handle must be called via defer at the top of the function:
//
//	func SomeFunction(ctx context.Context) {
//	    defer panics.Handle(ctx)
//	    // ... code that might panic
//	}
func Handle(ctx context.Context, handler ...PanicHandler) {
	handleWithContext(ctx, nil, handler...)
}

// panicShutdownTimeout is the maximum time to wait for shutdown hooks during panic recovery.
const panicShutdownTimeout = 2 * time.Second

// handleWithContext is an internal function that implements the core panic recovery logic.
func handleWithContext(ctx context.Context, opts *HandleOpts, handler ...PanicHandler) {
	if r := recover(); r != nil {
		// Run global shutdown hooks (best effort, short timeout)
		// We use a background context because the original context might be canceled or invalid
		// This ensures buffered logs are flushed even during panic
		shutdownCtx, cancel := context.WithTimeout(context.Background(), panicShutdownTimeout)
		coreruntime.RunShutdownHooks(shutdownCtx) //nolint:errcheck,contextcheck // best-effort during panic, original ctx may be invalid
		cancel()

		if handlers, ok := globalPanicHandlers.Load().(PanicHandlers); ok {
			for _, fn := range handlers {
				fn(ctx, r)
			}
		}

		for _, h := range handler {
			h(ctx, r)
		}

		rp := reallyPanic.Load()
		if opts != nil && opts.ReallyPanic != nil {
			rp = *opts.ReallyPanic
		}

		if rp {
			panic(r)
		}
	}
}

var globalPanicHandlers = atomic.Value{}
var panicLogger = atomic.Value{}
var loggerFromContext = atomic.Value{} // stores LoggerFromContextFunc

func init() {
	globalPanicHandlers.Store(clonePanicHandlers(PanicHandlers{panicLoggerHandler}))
}

func clonePanicHandlers(in PanicHandlers) PanicHandlers {
	return slices.Clone(in)
}

// AddGlobalPanicHandler appends handler to the global handler list. Global
// handlers run for every panic recovered by [Handle] or [HandleWithOpts],
// regardless of per-call handlers. Passing a nil handler is a no-op.
// AddGlobalPanicHandler is safe for concurrent use; an immutable copy of
// the handler slice is stored atomically on each call.
func AddGlobalPanicHandler(handler PanicHandler) {
	if handler == nil {
		return
	}

	var handlers PanicHandlers
	if existing, ok := globalPanicHandlers.Load().(PanicHandlers); ok && len(existing) > 0 {
		handlers = make(PanicHandlers, 0, len(existing)+1)
		handlers = append(handlers, existing...)
		handlers = append(handlers, handler)
	} else {
		handlers = PanicHandlers{handler}
	}

	// Store an immutable copy (no shared backing array) to avoid races and lost updates.
	globalPanicHandlers.Store(clonePanicHandlers(handlers))
}

// SetGlobalPanicHandlers atomically replaces all global handlers with the
// provided set. By default, a single handler that logs to [slog.Default] is
// installed at package init time. Pass an empty slice to disable global
// handling entirely. This function is safe for concurrent use.
func SetGlobalPanicHandlers(handlers ...PanicHandler) {
	// Store an immutable copy (no shared backing array).
	globalPanicHandlers.Store(clonePanicHandlers(handlers))
}

// SetReallyPanic controls the global re-panic behavior. When ok is true,
// [Handle] and [HandleWithOpts] (unless overridden by [HandleOpts.ReallyPanic])
// will re-panic after executing all handlers, allowing the panic to propagate
// up the call stack. The default is false (panics are swallowed).
// SetReallyPanic is safe for concurrent use.
func SetReallyPanic(ok bool) {
	reallyPanic.Store(ok)
}

// SetLogger sets the [slog.Logger] used by the built-in panic handler for
// logging recovered panics. If not called, the handler falls back to a
// context-derived logger (see [SetLoggerFromContext]) and then [slog.Default].
// SetLogger is safe for concurrent use.
func SetLogger(l *slog.Logger) {
	panicLogger.Store(l)
}

// SetLoggerFromContext registers fn as the strategy for extracting an
// [slog.Logger] from a context in the built-in panic handler. The handler
// tries fn first; if fn returns nil it falls back to the logger set via
// [SetLogger], and finally to [slog.Default].
//
// Example:
//
//	panics.SetLoggerFromContext(func(ctx context.Context) *slog.Logger {
//	    return myapp.LoggerFromContext(ctx)
//	})
//
// SetLoggerFromContext is safe for concurrent use.
func SetLoggerFromContext(fn LoggerFromContextFunc) {
	loggerFromContext.Store(fn)
}

// panicLoggerHandler is the default panic handler that logs the panic details.
// It captures a stack trace and logs the panic value along with it.
// The logger is selected in the following priority order:
// 1. Logger from context (if SetLoggerFromContext was called)
// 2. Global logger (if SetLogger was called)
// 3. slog.Default()
func panicLoggerHandler(ctx context.Context, r any) {
	// Manually allocate stack trace buffer size to prevent excessively large logs.
	const size = 64 << 10
	stacktrace := make([]byte, size)
	stacktrace = stacktrace[:runtime.Stack(stacktrace, false)]

	logger := slog.Default()

	// Try to get logger from context using the configured function
	if fn, ok := loggerFromContext.Load().(LoggerFromContextFunc); ok && fn != nil {
		if ctxLogger := fn(ctx); ctxLogger != nil {
			logger = ctxLogger
		}
	}

	// Fall back to global logger if context logger is not available
	if logger == slog.Default() {
		if globalLogger := panicLogger.Load(); globalLogger != nil {
			if l, ok := globalLogger.(*slog.Logger); ok {
				logger = l
			}
		}
	}

	panicString := ""
	if str, ok := r.(string); ok {
		panicString = str
	} else {
		panicString = fmt.Sprintf("%#v", r)
	}

	attrs := []any{
		"panic", panicString,
		"stacktrace", string(stacktrace),
	}

	logger.Error("observed a panic", attrs...)
}
