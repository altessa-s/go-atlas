// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// maxDispatchDepth bounds how deeply nested Publish calls may go within a single
// dispatch chain. Genuine linear cascades (A handler publishes B, B publishes C)
// stay well under it; exceeding it signals an unbounded recursion.
const maxDispatchDepth = 32

var (
	// ErrNoHandler is reported by Validate for a required event type that has no
	// registered handler. It wraps per-type detail and is matchable with errors.Is.
	ErrNoHandler = errors.New("eventbus: required event type has no handler")

	// ErrEventCycle is returned by Publish when an event type is published again
	// while it is already being dispatched in the same chain (a re-entrant cycle,
	// e.g. handler A publishes B whose handler publishes A).
	ErrEventCycle = errors.New("eventbus: event cycle detected")

	// ErrDispatchTooDeep is returned by Publish when nested publishing exceeds
	// maxDispatchDepth, guarding against unbounded recursion.
	ErrDispatchTooDeep = errors.New("eventbus: dispatch depth limit exceeded")
)

// handlersList holds the regular handlers and adapters for a specific event type.
type handlersList struct {
	handlers []Handler // business-logic handlers
	adapters []Handler // adapters (run after all handlers)
}

// dispatchStateKey is the context key under which the current dispatch chain's
// state is carried, so nested Publish calls can detect cycles and depth.
type dispatchStateKey struct{}

// dispatchState is one level of the dispatch chain: the event type being
// dispatched and the level that published it. The links never change once
// built, so a handler may publish from several goroutines sharing its context:
// each nested Publish links its own level to the parent.
//
// done is set when the level's dispatch returns. A context outlives the
// dispatch that created it whenever a handler retains it, so a completed level
// stays reachable; it no longer counts as in flight or toward the depth.
type dispatchState struct {
	parent    *dispatchState
	eventType reflect.Type
	done      atomic.Bool
}

// InProcessBus is a lock-free, synchronous, transaction-safe event bus. It uses
// atomic copy-on-write for lock-free reads on the hot Publish path.
type InProcessBus struct {
	handlers       atomic.Value // map[reflect.Type]*handlersList
	globalAdapters atomic.Value // []Handler (run for every event)
	required       atomic.Value // map[reflect.Type]struct{} (event types that must have a handler)
	mu             sync.Mutex   // serializes Subscribe/RegisterAdapter/RequireHandler writes

	options
}

var _ Bus = (*InProcessBus)(nil)

// New creates an empty in-process event bus. Pass options such as WithTxProbe
// to configure transaction-awareness.
func New(opts ...Option) *InProcessBus {
	eb := &InProcessBus{options: *newOptions(opts...)}
	eb.handlers.Store(make(map[reflect.Type]*handlersList))
	eb.globalAdapters.Store([]Handler{})
	eb.required.Store(make(map[reflect.Type]struct{}))
	return eb
}

// Subscribe registers a handler for an event type. Handlers run before adapters.
func (eb *InProcessBus) Subscribe(eventType any, handler Handler) {
	eb.modifyHandlersList(eventType, func(list *handlersList) {
		list.handlers = append(list.handlers, handler)
	})
}

// RegisterAdapter registers an adapter for an event type. Adapters run after all
// regular handlers (suitable for side effects such as broker publishing).
func (eb *InProcessBus) RegisterAdapter(eventType any, adapter Handler) {
	eb.modifyHandlersList(eventType, func(list *handlersList) {
		list.adapters = append(list.adapters, adapter)
	})
}

// RegisterGlobalAdapter registers an adapter that runs for every event type,
// after all type-specific handlers and adapters.
func (eb *InProcessBus) RegisterGlobalAdapter(adapter Handler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	current := eb.loadGlobalAdapters()
	newSlice := slices.Clone(current)
	newSlice = append(newSlice, adapter)
	eb.globalAdapters.Store(newSlice)
}

// RequireHandler records that the given event type must have at least one
// handler by the time Validate runs. eventType is a zero-value instance of the
// event type; a nil (untyped) value is ignored. Declaring a requirement turns a
// missing subscription from a silent no-op into a startup failure.
func (eb *InProcessBus) RequireHandler(eventType any) {
	t := reflect.TypeOf(eventType)
	if t == nil {
		return
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()

	current := eb.loadRequired()
	if _, ok := current[t]; ok {
		return
	}
	newMap := maps.Clone(current)
	newMap[t] = struct{}{}
	eb.required.Store(newMap)
}

// Validate returns an error if any type registered via RequireHandler has no
// handler. Adapters do not satisfy a requirement: a required event must have a
// business handler. Call it once after all subscriptions are wired (e.g. from an
// fx OnStart hook) so a mis-wired graph fails fast instead of dropping events.
// Missing types are reported as a joined error; each wraps ErrNoHandler.
func (eb *InProcessBus) Validate() error {
	handlersMap := eb.loadHandlers()

	required := eb.loadRequired()
	missing := make([]string, 0, len(required))
	for t := range required {
		if list := handlersMap[t]; list == nil || len(list.handlers) == 0 {
			missing = append(missing, t.String())
		}
	}

	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing) // stable, deterministic error message
	var joined error
	for _, name := range missing {
		joined = errors.Join(joined, fmt.Errorf("%w: %s", ErrNoHandler, name))
	}
	return joined
}

// Publish runs, synchronously and in order, the type-specific handlers, the
// type-specific adapters, then the global adapters. It stops at the first error.
// A nil event matches no type-specific handlers but still runs global adapters.
// Re-entrant publishing of an in-flight type fails with ErrEventCycle, and
// excessive nesting fails with ErrDispatchTooDeep.
func (eb *InProcessBus) Publish(ctx context.Context, event any) error {
	_, err := eb.dispatch(ctx, event)
	return err
}

// loadHandlers returns the current handlers map. The atomic is seeded by New and
// only ever stores this map type; a failed assertion signals a programming error
// and must panic.
func (eb *InProcessBus) loadHandlers() map[reflect.Type]*handlersList {
	m, ok := eb.handlers.Load().(map[reflect.Type]*handlersList)
	if !ok {
		panic(fmt.Sprintf("eventbus: handlers atomic holds unexpected type %T", eb.handlers.Load()))
	}
	return m
}

// loadGlobalAdapters returns the current global adapters. The atomic is seeded by
// New and only ever stores this slice type; a failed assertion signals a
// programming error and must panic.
func (eb *InProcessBus) loadGlobalAdapters() []Handler {
	s, ok := eb.globalAdapters.Load().([]Handler)
	if !ok {
		panic(fmt.Sprintf("eventbus: globalAdapters atomic holds unexpected type %T", eb.globalAdapters.Load()))
	}
	return s
}

// loadRequired returns the current required-types set. The atomic is seeded by
// New and only ever stores this map type; a failed assertion signals a
// programming error and must panic.
func (eb *InProcessBus) loadRequired() map[reflect.Type]struct{} {
	m, ok := eb.required.Load().(map[reflect.Type]struct{})
	if !ok {
		panic(fmt.Sprintf("eventbus: required atomic holds unexpected type %T", eb.required.Load()))
	}
	return m
}

// modifyHandlersList performs a copy-on-write update of the handlers map.
func (eb *InProcessBus) modifyHandlersList(eventType any, modifier func(*handlersList)) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	t := reflect.TypeOf(eventType)

	current := eb.loadHandlers()
	newMap := make(map[reflect.Type]*handlersList, len(current)+1)
	for k, v := range current {
		newMap[k] = &handlersList{
			handlers: slices.Clone(v.handlers),
			adapters: slices.Clone(v.adapters),
		}
	}
	if newMap[t] == nil {
		newMap[t] = &handlersList{}
	}
	modifier(newMap[t])
	eb.handlers.Store(newMap)
}

// enterDispatch records entry into the dispatch of type t in the chain carried
// by ctx, returning a context to pass downstream and the new level, which the
// caller marks done when its dispatch returns. It fails with ErrEventCycle if t
// is still in flight in this chain, or ErrDispatchTooDeep if the depth limit is
// exceeded. Levels whose dispatch has returned are skipped. The links are never
// mutated: the returned context carries a new level linked to its parent.
func enterDispatch(ctx context.Context, t reflect.Type) (context.Context, *dispatchState, error) {
	parent, _ := ctx.Value(dispatchStateKey{}).(*dispatchState) //nolint:errcheck // nil outside a dispatch
	depth := 0
	for level := parent; level != nil; level = level.parent {
		if level.done.Load() {
			continue
		}
		// A nil event has the nil type, which is compared like any other: a
		// global adapter that publishes nil again is a cycle too.
		if level.eventType == t {
			return ctx, nil, fmt.Errorf("%w: %v", ErrEventCycle, t)
		}
		depth++
	}
	if depth >= maxDispatchDepth {
		return ctx, nil, fmt.Errorf("%w: depth %d", ErrDispatchTooDeep, depth)
	}
	level := &dispatchState{parent: parent, eventType: t}
	return context.WithValue(ctx, dispatchStateKey{}, level), level, nil
}

// Detach returns a context derived from ctx that carries no dispatch chain: a
// Publish with it starts a new chain, as if it were called outside any
// handler. Everything else in ctx — values, deadline, cancellation, an active
// transaction — is kept.
//
// Use it for asynchronous work a handler starts and does not wait for: a
// goroutine that publishes alongside or after the handler is not nested in
// its dispatch, yet with the handler's context it gets [ErrEventCycle] for
// every event type still being dispatched in that chain.
//
// A detached chain is invisible to the chain it came from: if the publish is
// in fact causally nested — the handler waits for it, or the follow-up
// triggers the original event again — the cycle is no longer detected and
// recursion is no longer bounded by [ErrDispatchTooDeep]. Do not use it to
// silence a cycle in synchronous code.
func Detach(ctx context.Context) context.Context {
	return context.WithValue(ctx, dispatchStateKey{}, (*dispatchState)(nil))
}

// dispatchResult summarizes one dispatch for observability: how many handlers
// and adapters ran, and—only when no handler ran—whether the event type was
// declared required via RequireHandler. A decorator uses it to surface silent
// non-delivery (see observedBus).
type dispatchResult struct {
	Handlers int
	Adapters int
	Required bool
}

// dispatcher is the optional capability, implemented by *InProcessBus, that lets
// a decorator obtain per-publish dispatch counts without widening the Bus
// interface. observedBus type-asserts its wrapped Bus to it.
type dispatcher interface {
	dispatch(ctx context.Context, event any) (dispatchResult, error)
}

// dispatch performs delivery (see Publish) and returns a summary for metrics.
func (eb *InProcessBus) dispatch(ctx context.Context, event any) (dispatchResult, error) {
	t := reflect.TypeOf(event)

	ctx, level, err := enterDispatch(ctx, t)
	if err != nil {
		return dispatchResult{}, err
	}
	defer level.done.Store(true)

	list := eb.loadHandlers()[t]

	var res dispatchResult
	if list != nil {
		for i, handler := range list.handlers {
			if err := handler(ctx, event); err != nil {
				return res, coreerrs.WrapOperationWithContext(err, "publish event", fmt.Sprintf("handler %d, event %T", i, event))
			}
			res.Handlers++
		}
		for i, adapter := range list.adapters {
			if err := adapter(ctx, event); err != nil {
				return res, coreerrs.WrapOperationWithContext(err, "publish event", fmt.Sprintf("adapter %d, event %T", i, event))
			}
			res.Adapters++
		}
	}

	for i, adapter := range eb.loadGlobalAdapters() {
		if err := adapter(ctx, event); err != nil {
			return res, coreerrs.WrapOperationWithContext(err, "publish event", fmt.Sprintf("global adapter %d, event %T", i, event))
		}
		res.Adapters++
	}

	// Consult the required set only when nothing handled the event — the sole
	// case the no-handler metric cares about. The read is lock-free.
	if res.Handlers == 0 {
		res.Required = eb.isRequired(t)
	}
	return res, nil
}

// isRequired reports whether t was declared via RequireHandler.
func (eb *InProcessBus) isRequired(t reflect.Type) bool {
	if t == nil {
		return false
	}
	_, ok := eb.loadRequired()[t]
	return ok
}
