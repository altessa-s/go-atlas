// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreio "github.com/altessa-s/go-atlas-core/io"
	probfilter "github.com/altessa-s/go-atlas/cache/probfilter"
)

// RevocationStorage defines a unified interface for checking and managing revoked items
// (tokens, jtis, or kids). It abstracts the underlying storage (e.g., Bloom filter, Redis).
type RevocationStorage interface {
	// IsRevoked checks if the given item (token, jti, or kid) is revoked.
	IsRevoked(ctx context.Context, item string) (bool, error)

	// MarkRevoked marks an item as revoked with an optional TTL.
	// This is typically called after a successful remote introspection that returns 'active: false'.
	// It is an optimization, not a durable record: a later Sync may drop items
	// its source does not contain, so the source must hold every revocation
	// that has to survive (the provider itself only marks items the IdP keeps
	// reporting as inactive).
	MarkRevoked(ctx context.Context, item string, ttl time.Duration) error

	// Sync performs a synchronization of revoked items from an external source.
	// NewProvider calls it once at construction; the provider's refresh cron
	// calls it on WithRevocationSyncSchedule.
	Sync(ctx context.Context) error
}

// Authoritative is the exact revocation store a probabilistic filter fronts. It
// is consulted only when the filter cannot rule an item out, so a filter hit is
// confirmed against the exact answer instead of being trusted on its own.
//
// Following the project convention the dependency is declared here, on the
// consumer side, so any exact store can be injected.
type Authoritative interface {
	// IsRevoked reports, authoritatively, whether item is revoked.
	IsRevoked(ctx context.Context, item string) (bool, error)
}

// RevocationSyncReporter is an optional interface of a [RevocationStorage]
// whose revocation set is shared between replicas. It lets the provider count
// a sync committed by a peer replica as fresh for
// [WithRevocationMaxStaleness] without trusting a sync it merely saw start.
// The built-in filter storage implements it for filters shared through Redis.
type RevocationSyncReporter interface {
	// LastCommittedSync reports the last sync committed by any replica:
	// committed identifies it — it changes only when a sync commits — and
	// age is how long ago it committed. Both must be measured on one clock,
	// the one that stamped the commit (the Redis server's for the built-in
	// storage), so that age does not depend on how that clock relates to the
	// provider's: the provider never compares committed with its own clock.
	// known is false when no committed sync is known. The provider calls it
	// from its scheduled sync job, never while validating a token.
	LastCommittedSync(ctx context.Context) (committed time.Time, age time.Duration, known bool, err error)
}

// filterRevocationStorage is a RevocationStorage implementation using probabilistic filters.
type filterRevocationStorage struct {
	filter        probfilter.Filter
	loader        probfilter.DataLoader
	authoritative Authoritative
	// allowEmpty makes an empty sync result authoritative; set by the provider
	// from WithRevocationAllowEmpty.
	allowEmpty bool
	// entries is the number of entries the last successful Sync loaded.
	entries atomic.Int64
}

// NewFilterRevocationStorage creates a new revocation storage using a
// probabilistic filter, optionally confirmed by an exact store.
//
// A probabilistic filter has no false negatives but does have false positives
// (the Bloom default is 1%, see probfilterconfig.BloomDefaults). It can
// therefore prove an item is NOT revoked, but never that it IS. When
// authoritative is non-nil every filter hit is confirmed against it, so a false
// positive costs one exact lookup and nothing else.
//
// Passing a nil authoritative selects the lossy mode: an unconfirmed filter hit
// is reported as revoked. That preserves the security invariant — a revoked item
// is never allowed — but rejects roughly falsePositiveRate of valid items. Wire
// an authoritative store for any deployment where that is not acceptable.
func NewFilterRevocationStorage(filter probfilter.Filter, loader probfilter.DataLoader, authoritative Authoritative) RevocationStorage {
	return &filterRevocationStorage{
		filter:        filter,
		loader:        loader,
		authoritative: authoritative,
	}
}

// IsRevoked implements RevocationStorage. A definite filter miss is exact and
// answered locally; a hit is confirmed against the authoritative store when one
// is configured, and otherwise reported as revoked — see
// [NewFilterRevocationStorage] for the trade-off.
func (s *filterRevocationStorage) IsRevoked(ctx context.Context, item string) (bool, error) {
	if s.filter == nil {
		return false, nil
	}

	might, err := s.filter.MightExist(ctx, item)
	if err != nil {
		return false, err
	}
	if !might {
		// A probabilistic filter has no false negatives, so a definite miss
		// is already the exact answer.
		return false, nil
	}
	if s.authoritative == nil {
		return true, nil
	}

	return s.authoritative.IsRevoked(ctx, item)
}

// MarkRevoked implements RevocationStorage.
func (s *filterRevocationStorage) MarkRevoked(ctx context.Context, item string, _ time.Duration) error {
	if s.filter == nil {
		return nil
	}
	return s.filter.Add(ctx, item)
}

// Compile-time guarantee that the filter storage forwards the provider's
// shared HTTP client to its loader (see [filterRevocationStorage.SetHTTPClient]).
var _ httpClientSetter = (*filterRevocationStorage)(nil)

// SetHTTPClient implements shared-client injection by forwarding the
// client to the loader when the loader accepts one (e.g. [URLRevocationLoader]).
// The loader keeps its own preserve-vs-overwrite policy. This lets
// [NewProvider] inject its shared client into a storage that was assembled
// outside the provider and passed via [WithRevocationStorage].
func (s *filterRevocationStorage) SetHTTPClient(c *http.Client) {
	if setter, ok := s.loader.(httpClientSetter); ok {
		setter.SetHTTPClient(c)
	}
}

// Sync implements RevocationStorage. A source that turns up empty while the
// revocation set holds entries is a failed sync ([ErrRevocationLoadFailed]),
// not an emptied list: the filter keeps its contents instead of un-revoking
// everything on an empty or cut-off response. [WithRevocationAllowEmpty]
// makes the empty result authoritative instead.
//
// Whether the list in force holds entries is read from the filter itself on
// every empty result (see holdsEntries), once the source has been read and — for a
// filter shared through Redis — while this rebuild holds the rebuild lease,
// so no peer can publish between the check and the commit.
func (s *filterRevocationStorage) Sync(ctx context.Context) error {
	if s.filter == nil || s.loader == nil {
		return nil
	}

	rebuildable, ok := s.filter.(probfilter.RebuildableFilter)
	if !ok {
		return coreerrs.Wrapf(ErrFilterNotRebuildable, "filter type %T", s.filter)
	}

	loader := &countingLoader{DataLoader: s.loader}
	if !s.allowEmpty {
		loader.holdsEntries = s.holdsEntries
	}
	if err := rebuildable.Rebuild(ctx, loader); err != nil {
		return err
	}
	s.entries.Store(loader.loaded)
	return nil
}

// holdsEntries reports whether the revocation list in force holds entries,
// that is, whether the last committed sync loaded any. A filter that records
// this ([probfilter.RebuildLoadReporter], as the Bloom and Cuckoo filters do)
// is asked every time: a filter shared through Redis may hold a list a peer
// replica published after this storage's own last sync. Items added with
// MarkRevoked are not part of the list and do not count, so an empty list
// stays empty for the guard whatever introspection added to the filter.
//
// When the size of the committed list is unknown — the filter was never
// rebuilt, was recreated since, or was committed by a release that did not
// record it — the filter's item count ([probfilter.StatsProvider]) decides
// instead, which also counts MarkRevoked items; a size or count that cannot
// be read counts as entries. Only a filter that reports neither falls back
// to what this storage's own last sync loaded. A Redis filter costs one
// round trip per empty sync result, an in-memory one nothing (and one
// bit-count pass for a Bloom filter that was never rebuilt).
func (s *filterRevocationStorage) holdsEntries(ctx context.Context) bool {
	if reporter, ok := s.filter.(probfilter.RebuildLoadReporter); ok {
		loaded, known, err := reporter.LastRebuildLoaded(ctx)
		if err != nil {
			return true
		}
		if known {
			return loaded > 0
		}
	}
	provider, ok := s.filter.(probfilter.StatsProvider)
	if !ok {
		return s.entries.Load() > 0
	}
	filterStats, err := provider.Stats(ctx)
	return err != nil || (filterStats != nil && filterStats.ItemCount > 0)
}

// Compile-time guarantee that the filter storage reports shared sync
// freshness to the provider.
var _ RevocationSyncReporter = (*filterRevocationStorage)(nil)

// LastCommittedSync implements [RevocationSyncReporter] with the filter's
// last committed rebuild and its age as the filter's own clock measures it
// ([probfilter.RebuildCommitAgeReporter]); a filter that cannot report one
// yields known == false.
func (s *filterRevocationStorage) LastCommittedSync(ctx context.Context) (committed time.Time, age time.Duration, known bool, err error) {
	reporter, ok := s.filter.(probfilter.RebuildCommitAgeReporter)
	if !ok {
		return time.Time{}, 0, false, nil
	}
	age, committed, known, err = reporter.RebuildCommitAge(ctx)
	return committed, age, known, err
}

// countingLoader counts the entries a rebuild streams and, with holdsEntries
// set, fails a stream that yields none while the revocation set holds some.
type countingLoader struct {
	probfilter.DataLoader
	// holdsEntries is consulted when the stream ended without an entry; the
	// rebuild is then still running, under its lease for a shared filter.
	holdsEntries func(ctx context.Context) bool
	loaded       int64
}

// StreamValues implements DataLoader.
func (l *countingLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		l.loaded = 0
		for v, err := range l.DataLoader.StreamValues(ctx) {
			if err == nil {
				l.loaded++
			}
			if !yield(v, err) {
				return
			}
		}
		if l.loaded == 0 && l.holdsEntries != nil && l.holdsEntries(ctx) {
			yield("", coreerrs.Wrap(ErrRevocationLoadFailed, "source returned no entries; keeping the current revocation set"))
		}
	}
}

// scanRevocationEntries yields the entries of a one-per-line revocation list:
// surrounding whitespace is trimmed and blank lines are skipped. A read
// failure — a line longer than [MaxRevocationEntryBytes], a body over the size
// cap — is yielded as the final error, so a cut-off list is never taken for a
// complete one.
func scanRevocationEntries(ctx context.Context, r io.Reader, yield func(string, error) bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, MaxRevocationEntryBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			yield("", err)
			return
		}
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" {
			continue
		}
		if !yield(entry, nil) {
			return
		}
	}

	switch err := scanner.Err(); {
	case err == nil:
	case errors.Is(err, bufio.ErrTooLong):
		yield("", coreerrs.Wrapf(ErrRevocationLoadFailed, "revocation list entry exceeds %d bytes", MaxRevocationEntryBytes))
	case errors.Is(err, coreio.ErrReadLimitExceeded):
		yield("", fmt.Errorf("%w: revocation list too large: %w", ErrRevocationLoadFailed, err))
	default:
		yield("", err)
	}
}

// FileRevocationLoader loads revoked items from a regular file (one per line).
// Surrounding whitespace is trimmed and blank lines are skipped; a line longer
// than [MaxRevocationEntryBytes] fails the load.
type FileRevocationLoader struct {
	Path string
}

func (l *FileRevocationLoader) open(ctx context.Context) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(l.Path, os.O_RDONLY|nonblockOpenFlag, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && info.Mode().IsRegular() {
		return f, nil
	}
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return nil, coreerrs.Wrap(ErrRevocationLoadFailed, "revocation source must be a regular file")
}

// StreamValues implements DataLoader.
func (l *FileRevocationLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		file, err := l.open(ctx)
		if err != nil {
			yield("", err)
			return
		}
		defer func() { _ = file.Close() }()

		scanRevocationEntries(ctx, file, yield)
	}
}

// Count implements DataLoader.
func (l *FileRevocationLoader) Count(ctx context.Context) (int64, error) {
	file, err := l.open(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	var count int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		count++
	}
	return count, scanner.Err()
}

// URLRevocationLoader loads revoked items from a remote URL (one per line).
//
// Client is optional when the loader is wired into an OIDC [Provider] —
// [NewProvider] clones the built-in loader before injecting its shared
// HTTP client via SetHTTPClient, so the revocation
// refresh reuses the same pool, retry policy and proxy resolver as
// discovery/JWKS/userinfo.
//
// Setting Client explicitly is still supported as an escape hatch for
// tests or callers using URLRevocationLoader outside of [Provider]. In
// that case StreamValues fails fast if Client is nil rather than
// silently falling back to http.DefaultClient — match the failure with
// [ErrLoaderClientNotConfigured] via [errors.Is].
//
// Surrounding whitespace is trimmed and blank lines are skipped. A body larger
// than MaxResponseBytes or a line longer than [MaxRevocationEntryBytes] fails
// the load with [ErrRevocationLoadFailed] instead of yielding a cut-off list.
type URLRevocationLoader struct {
	URL    string
	Client *http.Client
	// MaxResponseBytes caps the response body; zero or a negative value
	// selects [DefaultRevocationMaxResponseBytes].
	MaxResponseBytes int64
}

// Compile-time guarantee that URLRevocationLoader satisfies the
// HTTPClientSetter contract Provider relies on for shared-client
// injection. Renaming or removing SetHTTPClient breaks at build time
// instead of silently disabling the wiring at runtime.
var _ httpClientSetter = (*URLRevocationLoader)(nil)

// SetHTTPClient implements shared-client injection for URLRevocationLoader.
// It adopts the supplied client only when no Client was set explicitly,
// preserving the caller's escape-hatch override.
func (l *URLRevocationLoader) SetHTTPClient(c *http.Client) {
	if l.Client == nil {
		l.Client = c
	}
}

// A failed fetch also prevents a revocation check from being completed.
var errRevocationFetch = errors.Join(ErrRevocationLoadFailed, ErrRevocationCheck)

// StreamValues implements DataLoader.
func (l *URLRevocationLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if l.Client == nil {
			yield("", coreerrs.Wrap(ErrLoaderClientNotConfigured, "set the Client field or wire the loader via oidc.NewProvider"))
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
		if err != nil {
			yield("", boundaryError("load revocations", 0, errRevocationFetch, err))
			return
		}

		resp, err := l.Client.Do(req)
		if err != nil {
			yield("", boundaryError("load revocations", 0, errRevocationFetch, err))
			return
		}
		defer drainAndClose(resp)

		if resp.StatusCode != http.StatusOK {
			yield("", boundaryError("load revocations", resp.StatusCode, errRevocationFetch, nil))
			return
		}

		limit := l.MaxResponseBytes
		if limit <= 0 {
			limit = DefaultRevocationMaxResponseBytes
		}
		scanRevocationEntries(ctx, coreio.NewLimitedReadCloser(resp.Body, limit), func(entry string, err error) bool {
			if err != nil {
				err = boundaryError("read revocations", resp.StatusCode, errRevocationFetch, err)
			}
			return yield(entry, err)
		})
	}
}

// Count implements DataLoader.
func (l *URLRevocationLoader) Count(_ context.Context) (int64, error) {
	return -1, nil // Unknown count for URLs
}
