// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"

	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

var (
	errListFailed  = errors.New("list failed")
	errValueFailed = errors.New("value failed")
)

// dirtyTestProvider is a minimal provider whose List result is configurable.
type dirtyTestProvider struct {
	mu        sync.Mutex
	values    map[string]string
	listErr   error
	failValue bool   // Value fails while set
	afterList func() // runs once after a successful List, if set
}

func (p *dirtyTestProvider) Name() string { return "dirty-test" }

func (p *dirtyTestProvider) List(_ context.Context) ([]*Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listErr != nil {
		return nil, p.listErr
	}
	list := make([]*Value[string], 0, len(p.values))
	for k, v := range p.values {
		list = append(list, NewValue(k, v, nil, v))
	}
	if hook := p.afterList; hook != nil {
		p.afterList = nil
		p.mu.Unlock()
		hook()
		p.mu.Lock()
	}
	return list, nil
}

func (p *dirtyTestProvider) Values(context.Context) iter.Seq2[*Value[string], error] {
	return func(func(*Value[string], error) bool) {}
}

func (p *dirtyTestProvider) Value(_ context.Context, key string) (*Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failValue {
		return nil, errValueFailed
	}
	v, ok := p.values[key]
	if !ok {
		return nil, ErrNotFound
	}
	return NewValue(key, v, nil, v), nil
}

func (p *dirtyTestProvider) Save(_ context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values[key] = value
	return nil
}

func (p *dirtyTestProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.values, key)
	return nil
}

func (t *Manager[T]) cycleDirtyForTest() map[string]struct{} {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	return t.cycleDirty
}

// TestManager_CycleDirtyOnlyDuringCycle checks that cache writes are only
// tracked while an update cycle runs, whatever its outcome, so the tracking
// cannot grow without bound.
func TestManager_CycleDirtyOnlyDuringCycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &dirtyTestProvider{values: map[string]string{"k": "v"}}
	mgr, err := New[string](provider, WithMaxRetries(1))
	require.NoError(t, err)

	require.NoError(t, mgr.Save(ctx, "a", "1"))
	require.NoError(t, mgr.Delete(ctx, "a"))
	_, err = mgr.Value(ctx, "k", true)
	require.NoError(t, err)
	require.Nil(t, mgr.cycleDirtyForTest(), "no tracking outside a cycle")

	provider.mu.Lock()
	provider.listErr = errListFailed
	provider.mu.Unlock()
	require.ErrorIs(t, mgr.RunUpdateCycle(ctx), errListFailed)
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after a failed cycle")

	provider.mu.Lock()
	provider.listErr = nil
	provider.values = map[string]string{}
	provider.mu.Unlock()
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after an empty cycle")

	require.NoError(t, mgr.Save(ctx, "b", "2"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after a successful cycle")
}

// signalProvider wraps dirtyTestProvider and signals once its Value call
// returned.
type signalProvider struct {
	*dirtyTestProvider
	returned chan struct{}
	once     sync.Once
}

func (p *signalProvider) Value(ctx context.Context, key string) (*Value[string], error) {
	v, err := p.dirtyTestProvider.Value(ctx, key)
	p.once.Do(func() { close(p.returned) })
	return v, err
}

// TestManager_FetchDoesNotOverwriteSaveBetweenCheckAndWrite holds cacheMu
// while a fetch of an old value completes, then publishes a Save of a newer
// value exactly as Save does (generation bump, then cache write) before
// releasing it: the fetch, which checks the Save generation only once it
// holds cacheMu, must not overwrite the newer value.
func TestManager_FetchDoesNotOverwriteSaveBetweenCheckAndWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &signalProvider{
		dirtyTestProvider: &dirtyTestProvider{values: map[string]string{"k": "old"}},
		returned:          make(chan struct{}),
	}
	mgr, err := New[string](provider)
	require.NoError(t, err)

	mgr.cacheMu.Lock()
	done := make(chan error, 1)
	go func() {
		// ValueShared: its cache lookup does not take cacheMu, so the fetch
		// runs up to its cache write.
		_, err := mgr.ValueShared(ctx, "k", true)
		done <- err
	}()
	<-provider.returned
	// Let the fetch run up to the cache write, which waits for cacheMu.
	time.Sleep(50 * time.Millisecond)
	// The fetch holds its reference to the key's state, so this is the
	// entry whose generation it snapshotted.
	st := mgr.acquireKey("k")
	st.gen.Add(1)
	mgr.releaseKey("k", st)
	mgr.cache.Put("k", NewValue("k", "new", nil, "new"))
	mgr.cacheMu.Unlock()
	require.NoError(t, <-done)

	got, err := mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "new", got.Value)
}

// gatedProvider wraps dirtyTestProvider and pauses one Value call, armed
// by the test: the call reads storage, signals read, and waits for release
// before returning what it read.
type gatedProvider struct {
	*dirtyTestProvider
	mu      sync.Mutex
	armed   bool
	read    chan struct{}
	release chan struct{}
}

func (p *gatedProvider) arm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed = true
	p.read = make(chan struct{})
	p.release = make(chan struct{})
}

func (p *gatedProvider) Value(ctx context.Context, key string) (*Value[string], error) {
	v, err := p.dirtyTestProvider.Value(ctx, key)
	p.mu.Lock()
	gated := p.armed
	p.armed = false
	read, release := p.read, p.release
	p.mu.Unlock()
	if gated {
		close(read)
		<-release
	}
	return v, err
}

// TestManager_FetchDoesNotResurrectDeletedKey pauses a fetch after it read
// the key until Delete completed: the fetch must not cache the deleted key.
func TestManager_FetchDoesNotResurrectDeletedKey(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &gatedProvider{dirtyTestProvider: &dirtyTestProvider{values: map[string]string{"k": "v"}}}
	mgr, err := New[string](provider)
	require.NoError(t, err)

	provider.arm()
	done := make(chan error, 1)
	go func() {
		_, err := mgr.Value(ctx, "k", true)
		done <- err
	}()
	<-provider.read
	require.NoError(t, mgr.Delete(ctx, "k"))
	close(provider.release)
	require.NoError(t, <-done)

	_, err = mgr.Value(ctx, "k", false)
	require.ErrorIs(t, err, ErrNotFound, "a fetch that read before Delete must not re-cache the key")
}

// TestManager_SaveReadbackDoesNotOverwriteNewerSave pauses Save A's readback
// and starts Save B of the same key: B must wait for A, and the cache must end
// up holding B's value, as the provider does.
func TestManager_SaveReadbackDoesNotOverwriteNewerSave(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &gatedProvider{dirtyTestProvider: &dirtyTestProvider{values: map[string]string{}}}
	mgr, err := New[string](provider)
	require.NoError(t, err)

	provider.arm()
	done := make(chan error, 1)
	go func() { done <- mgr.Save(ctx, "k", "v1") }()
	<-provider.read
	doneB := make(chan error, 1)
	go func() { doneB <- mgr.Save(ctx, "k", "v2") }()
	select {
	case err := <-doneB:
		t.Fatalf("Save B completed while Save A of the same key was in progress: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(provider.release)
	require.NoError(t, <-done)
	require.NoError(t, <-doneB)

	got, err := mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	stored, err := provider.dirtyTestProvider.Value(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "v2", stored.Value)
	require.Equal(t, stored.Value, got.Value, "a stale Save readback must not overwrite a newer Save")
}

// gatedDeleteProvider pauses its first Delete after the storage delete until
// released.
type gatedDeleteProvider struct {
	*dirtyTestProvider
	once    sync.Once
	deleted chan struct{}
	release chan struct{}
}

func (p *gatedDeleteProvider) Delete(ctx context.Context, key string) error {
	err := p.dirtyTestProvider.Delete(ctx, key)
	p.once.Do(func() {
		close(p.deleted)
		<-p.release
	})
	return err
}

// deletableOnly hides Rebuild, so the Manager treats the filter as a
// non-rebuildable deletable filter.
type deletableOnly struct {
	probfilter.DeletableFilter
}

// TestManager_SaveWaitsForConcurrentDelete pauses Delete right after the
// storage delete and starts a Save of the same key: the Save must wait for
// the Delete to finish, so storage, cache and negative filter all end up
// holding the saved key.
func TestManager_SaveWaitsForConcurrentDelete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &gatedDeleteProvider{
		dirtyTestProvider: &dirtyTestProvider{values: map[string]string{"k": "v1"}},
		deleted:           make(chan struct{}),
		release:           make(chan struct{}),
	}
	filter := deletableOnly{cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))}
	require.NoError(t, filter.Add(ctx, "k"))
	mgr, err := New[string](provider, WithNegativeFilter(filter))
	require.NoError(t, err)

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- mgr.Delete(ctx, "k") }()
	<-provider.deleted

	saveDone := make(chan error, 1)
	go func() { saveDone <- mgr.Save(ctx, "k", "v2") }()
	select {
	case err := <-saveDone:
		t.Fatalf("Save completed while Delete of the same key was in progress: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(provider.release)
	require.NoError(t, <-deleteDone)
	require.NoError(t, <-saveDone)

	stored, err := provider.dirtyTestProvider.Value(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "v2", stored.Value)
	cached, err := mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "v2", cached.Value)
	ok, err := filter.MightExist(ctx, "k")
	require.NoError(t, err)
	require.True(t, ok)
}

// TestManager_FetchAfterMutationDoesNotJoinEarlierFetch keeps a fetch that
// read before a Delete or Save blocked, then fetches again after the mutation
// completed: the new fetch must see the mutation, not join the earlier read.
func TestManager_FetchAfterMutationDoesNotJoinEarlierFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(context.Context, *Manager[string]) error
		check  func(*testing.T, *Value[string], error)
	}{
		{
			name:   "delete",
			mutate: func(ctx context.Context, m *Manager[string]) error { return m.Delete(ctx, "k") },
			check: func(t *testing.T, _ *Value[string], err error) {
				t.Helper()
				require.ErrorIs(t, err, ErrNotFound)
			},
		},
		{
			name: "save",
			mutate: func(ctx context.Context, m *Manager[string]) error {
				if err := m.Save(ctx, "k", "v2"); err != nil {
					return err
				}
				m.ClearCache(ctx) // force the next lookup to fetch
				return nil
			},
			check: func(t *testing.T, v *Value[string], err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, "v2", v.Value)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			provider := &gatedProvider{dirtyTestProvider: &dirtyTestProvider{values: map[string]string{"k": "v1"}}}
			mgr, err := New[string](provider, fastRetry()...)
			require.NoError(t, err)

			provider.arm()
			earlier := make(chan error, 1)
			go func() {
				_, err := mgr.Value(ctx, "k", true)
				earlier <- err
			}()
			<-provider.read
			require.NoError(t, tc.mutate(ctx, mgr))

			got, err := mgr.Value(ctx, "k", true)
			tc.check(t, got, err)

			close(provider.release)
			require.NoError(t, <-earlier)
		})
	}
}

// fastRetry keeps failing provider calls from backing off for seconds.
func fastRetry() []Option {
	return []Option{
		WithMaxRetries(1),
		WithExponentialConfig(retry.ExponentialConfig{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Factor: 1}),
	}
}

// TestManager_MutationLockHonorsContext holds a key's mutation lock and checks
// that a Save or Delete of the key waiting for it returns when its context
// ends, before the lock is released.
func TestManager_MutationLockHonorsContext(t *testing.T) {
	t.Parallel()

	provider := &dirtyTestProvider{values: map[string]string{"k": "v"}}
	mgr, err := New[string](provider)
	require.NoError(t, err)

	st := mgr.acquireKey("k")
	defer mgr.releaseKey("k", st)
	require.NoError(t, st.lock(t.Context()))
	defer st.unlock()

	for name, mutate := range map[string]func(context.Context) error{
		"save":   func(ctx context.Context) error { return mgr.Save(ctx, "k", "v2") },
		"delete": func(ctx context.Context) error { return mgr.Delete(ctx, "k") },
	} {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		err := mutate(ctx)
		cancel()
		require.ErrorIs(t, err, context.DeadlineExceeded, name)
	}

	stored, err := provider.Value(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, "v", stored.Value, "a canceled mutation must not reach the provider")
}

// TestManager_SaveWithFailedReadbackDropsOldValue saves while the readback
// fails, during an update cycle whose listing predates the Save: neither a
// cache hit nor the stale listing may serve the old value afterwards.
func TestManager_SaveWithFailedReadbackDropsOldValue(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &dirtyTestProvider{values: map[string]string{"k": "v1", "other": "o"}}
	mgr, err := New[string](provider, fastRetry()...)
	require.NoError(t, err)
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	provider.mu.Lock()
	provider.afterList = func() {
		provider.mu.Lock()
		provider.failValue = true
		provider.mu.Unlock()
		require.NoError(t, mgr.Save(ctx, "k", "v2"))
	}
	provider.mu.Unlock()
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	_, err = mgr.Value(ctx, "k", false)
	require.ErrorIs(t, err, ErrNotFound, "the old value must not stay cached after a successful Save")

	provider.mu.Lock()
	provider.failValue = false
	provider.mu.Unlock()
	got, err := mgr.Value(ctx, "k", true)
	require.NoError(t, err)
	require.Equal(t, "v2", got.Value)
}

// clonerProvider serves a privateCloner payload behind any.
type clonerProvider struct{ dirtyTestProvider }

func (p *clonerProvider) Value(_ context.Context, key string) (*Value[any], error) {
	return NewValue[any](key, privateCloner{secret: []byte("s")}, nil, "1"), nil
}

func (p *clonerProvider) List(context.Context) ([]*Value[any], error) { return nil, nil }

func (p *clonerProvider) Values(context.Context) iter.Seq2[*Value[any], error] {
	return func(func(*Value[any], error) bool) {}
}

func (p *clonerProvider) Save(context.Context, string, any) error { return nil }

// TestManager_AnyPayloadWithPrivateCloner checks that a Manager[any] payload
// with private state and its own Clone method is isolated between callers.
func TestManager_AnyPayloadWithPrivateCloner(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	mgr, err := New[any](&clonerProvider{})
	require.NoError(t, err)

	fetched, err := mgr.Value(ctx, "k", true)
	require.NoError(t, err)
	fetched.Value.(privateCloner).set('X')

	hit, err := mgr.Value(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "s", string(hit.Value.(privateCloner).secret))
	hit.Value.(privateCloner).set('Y')

	cached, err := mgr.ValueShared(ctx, "k", false)
	require.NoError(t, err)
	require.Equal(t, "s", string(cached.Value.(privateCloner).secret))
}
