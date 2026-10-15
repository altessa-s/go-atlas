// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Offset identifies a record in the WAL by segment ID and byte position.
type Offset struct {
	SegmentID uint64
	Position  int64
}

// IsZero reports whether the offset is the zero value, used to indicate
// "no WAL record" (e.g. when a dispatcher has WAL disabled).
func (o Offset) IsZero() bool { return o.SegmentID == 0 && o.Position == 0 }

const (
	fileSuffix       = ".wal"
	recordHeaderSize = 8 // 4 bytes length + 4 bytes crc32
	maxRecordBytes   = 64 << 20

	dirMode  os.FileMode = 0o700 // WAL directory permissions.
	fileMode os.FileMode = 0o600 // WAL segment file permissions.
)

// Sentinel errors.
var (
	// ErrInsecurePermissions means a WAL directory or segment grants group or
	// other users access. Restrict its permissions before reopening, or explicitly
	// opt out with WithInsecurePermissions.
	ErrInsecurePermissions = errors.New("wal: insecure permissions")
	// ErrClosed is returned by operations on a closed WAL.
	ErrClosed = errors.New("wal: closed")
	// ErrFull is returned by Append when the max-bytes cap is exceeded.
	ErrFull = errors.New("wal: full")
	// ErrEmptyPayload is returned by Append for zero-length payloads. The
	// on-disk format cannot represent them: recovery treats a zero record
	// length as corruption, so accepting one would silently truncate every
	// record written after it in the same segment on the next Open.
	ErrEmptyPayload = errors.New("wal: empty payload")
	// ErrPayloadTooLarge is returned by Append when the payload exceeds the
	// per-record limit enforced during recovery (64 MiB); writing it would
	// make the segment unrecoverable past that record.
	ErrPayloadTooLarge = errors.New("wal: payload too large")
)

// Record is a record found during WAL recovery.
type Record struct {
	Offset  Offset
	Payload []byte
}

// Stats is a snapshot of WAL state, used for metrics.
type Stats struct {
	TotalBytes int64
	Segments   int
	// Pending is the number of records not yet acknowledged in this process.
	Pending int64
}

// segmentRef tracks a single segment file and its outstanding (unacked) record count.
// The counters are atomic so the group-commit leader can publish a written
// batch without w.mu; every other field is guarded by w.mu, and file and the
// active pointer change only while holding both w.wmu and w.mu.
type segmentRef struct {
	id              uint64
	path            string
	file            *os.File     // nil for sealed segments
	size            atomic.Int64 // bytes of fully written records
	records         atomic.Int64 // total records ever appended to this segment
	unacked         atomic.Int64 // records not yet acked by consumer
	acked           map[int64]struct{}
	pendingPosition int64
}

// WAL is a segmented append-only log providing crash-safe durability for
// opaque byte payloads. Records are written length-prefixed with a CRC32;
// a background goroutine fsyncs the active segment on a fixed interval.
//
// WAL is safe for concurrent calls to Append, Ack, Sync, Stats, and Close.
type WAL struct {
	dir  string
	opts *options

	// mu guards segment state for readers, Ack and Stats. wmu serializes
	// appends; lock order is wmu before mu.
	mu     sync.Mutex
	wmu    sync.Mutex
	active *segmentRef   // current writable segment
	sealed []*segmentRef // sealed segments waiting for full ack
	nextID uint64
	// Group commit, guarded by wmu: Append encodes into cur, and one leader
	// at a time writes a whole batch outside the lock while the next one
	// fills. Records of cur belong to generation curGen; doneGen is the last
	// generation whose results are published. flushed is signaled with wmu.
	cur      *appendBatch
	curGen   uint64
	doneGen  uint64
	flushing bool
	flushed  sync.Cond
	// readBuf is the ReadPending read window, guarded by mu and allocated on
	// first use (pendingReadWindow bytes).
	readBuf []byte
	totalSz atomic.Int64
	closed  atomic.Bool
	closing atomic.Bool // single-winner guard for Close; see Close

	fsyncDone chan struct{}
	fsyncWG   sync.WaitGroup

	// lastFsyncErr captures the most recent background fsync failure
	// so callers can observe whether the WAL is in a "logged but
	// silent" failure state via [WAL.Faulted]. Previously fsync errors
	// were logged and forgotten — a durability primitive that loses
	// an fsync should not be invisible to consumers who care about
	// the guarantee. Stored as atomic.Pointer to keep reads lock-free
	// and the hot append/fsync paths uncoupled.
	lastFsyncErr atomic.Pointer[error]
}

// Open opens (or creates) a WAL in dir, recovering any existing segment
// files. The returned slice contains every valid record found, in the
// order they were originally written; callers must replay them and call
// Ack as records are processed.
//
// dir is created with 0700 and segments with 0600. Existing Unix paths
// granting group/other access are rejected with ErrInsecurePermissions.
// WithInsecurePermissions restores the legacy 0755/0644 behavior.
// Ownership, ancestor paths and ACLs remain the caller's responsibility.
// Additional tunables are
// supplied via [Option] values (see [WithMaxSegmentBytes], [WithMaxBytes]
// and [WithFsyncInterval]).
//
// A background fsync goroutine is started before Open returns.
func Open(dir string, opts ...Option) (*WAL, []Record, error) {
	if dir == "" {
		return nil, nil, errors.New("wal: dir is required")
	}
	resolved := newOptions(opts...)
	mode := dirMode
	if resolved.insecurePermissions {
		mode = 0o755
	}
	if err := os.MkdirAll(dir, mode); err != nil {
		return nil, nil, coreerrs.Wrap(err, "wal: mkdir")
	}

	if err := validatePath(dir, true, resolved.insecurePermissions); err != nil {
		return nil, nil, err
	}

	w := &WAL{
		dir:       dir,
		opts:      resolved,
		fsyncDone: make(chan struct{}),
		nextID:    1, // 0 is reserved for "no record" so the zero Offset is unambiguous.
		cur:       getAppendBatch(),
		curGen:    1,
	}
	w.flushed.L = &w.wmu

	recovered, err := w.recover()
	if err != nil {
		return nil, nil, err
	}

	if err := w.openNewActiveLocked(); err != nil {
		return nil, nil, err
	}

	w.fsyncWG.Go(w.fsyncLoop)

	return w, recovered, nil
}

func (w *WAL) recover() ([]Record, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, coreerrs.Wrap(err, "wal: read dir")
	}

	type segFile struct {
		id   uint64
		path string
	}
	var segs []segFile
	for _, e := range entries {
		name := e.Name()
		idStr, ok := strings.CutSuffix(name, fileSuffix)
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseUint(idStr, 10, 64)
		if parseErr != nil {
			continue
		}
		segs = append(segs, segFile{id: id, path: filepath.Join(w.dir, name)})
	}
	slices.SortFunc(segs, func(a, b segFile) int { return cmp.Compare(a.id, b.id) })

	// Preflight every target before recovery can delete or truncate any file.
	for _, sf := range segs {
		if err := validatePath(sf.path, false, w.opts.insecurePermissions); err != nil {
			return nil, err
		}
	}
	var recovered []Record
	for _, sf := range segs {
		recs, validSize, readErr := readSegment(sf.path)
		if readErr != nil {
			return nil, readErr
		}
		if len(recs) == 0 {
			// Empty / corrupt-from-start segment: remove it. Log
			// failures (EACCES, EBUSY, read-only mount) rather than
			// silently swallowing them — those signal an operator
			// misconfiguration the WAL would otherwise hide.
			if rmErr := os.Remove(sf.path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				w.opts.logger.Warn("wal: failed to remove empty/corrupt segment on recovery",
					slog.String("path", sf.path),
					slog.Any("error", rmErr),
				)
			}
			continue
		}
		// Truncate trailing torn record(s).
		if info, statErr := os.Stat(sf.path); statErr == nil && info.Size() != validSize {
			if err := os.Truncate(sf.path, validSize); err != nil {
				return nil, coreerrs.Wrapf(err, "wal: truncate %s", sf.path)
			}
		}
		ref := &segmentRef{id: sf.id, path: sf.path}
		ref.size.Store(validSize)
		ref.records.Store(int64(len(recs)))
		ref.unacked.Store(int64(len(recs)))
		w.sealed = append(w.sealed, ref)
		w.totalSz.Add(validSize)
		if sf.id >= w.nextID {
			w.nextID = sf.id + 1
		}
		for _, r := range recs {
			recovered = append(recovered, Record{
				Offset:  Offset{SegmentID: sf.id, Position: r.position},
				Payload: r.payload,
			})
		}
	}
	return recovered, nil
}

// validatePath follows symlinks under the caller-owned path trust boundary.
// Windows callers must enforce access through ACLs rather than POSIX mode bits.
func validatePath(path string, directory, insecure bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return coreerrs.Wrap(err, "wal: stat path")
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return coreerrs.Wrap(os.ErrInvalid, "wal: expected directory or regular segment")
	}
	if !insecure && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return coreerrs.Wrapf(ErrInsecurePermissions, "wal: %s has permissions %04o", path, info.Mode().Perm())
	}
	return nil
}

type rawRecord struct {
	position int64
	payload  []byte
}

// readSegment reads all valid records in path, stopping at the first
// corrupt or partial record. Returns (records, validBytes, err).
func readSegment(path string) ([]rawRecord, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, coreerrs.Wrapf(err, "wal: open %s", path)
	}
	defer f.Close() //nolint:errcheck // read-only

	var (
		recs   []rawRecord
		offset int64
		hdr    [recordHeaderSize]byte
	)
	for {
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			// EOF (clean) or partial header (torn write): stop.
			break
		}
		length := binary.LittleEndian.Uint32(hdr[0:4])
		crc := binary.LittleEndian.Uint32(hdr[4:8])
		if length == 0 || length > maxRecordBytes {
			// Implausible length, treat as corruption.
			break
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(f, payload); err != nil {
			// Torn write.
			break
		}
		if crc32.ChecksumIEEE(payload) != crc {
			// Corruption.
			break
		}
		recs = append(recs, rawRecord{position: offset, payload: payload})
		offset += int64(recordHeaderSize) + int64(length)
	}
	return recs, offset, nil
}

// openNewActiveLocked opens a fresh active segment file. Caller need not
// hold the locks when called from Open (single-threaded), but must hold
// w.wmu and w.mu when called during a roll.
func (w *WAL) openNewActiveLocked() error {
	id := w.nextID
	w.nextID++
	name := fmt.Sprintf("%020d%s", id, fileSuffix)
	path := filepath.Join(w.dir, name)
	mode := fileMode
	if w.opts.insecurePermissions {
		mode = 0o644
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return coreerrs.Wrap(err, "wal: open active")
	}
	w.active = &segmentRef{id: id, path: path, file: f}
	return nil
}

// Append writes payload to the active segment and returns its offset. The
// data lands in the OS page cache; durability is provided by the periodic
// fsync goroutine. Returns ErrClosed after Close, ErrFull when MaxBytes
// is configured and exceeded, ErrEmptyPayload for zero-length payloads,
// or ErrPayloadTooLarge for payloads over 64 MiB — the latter two cannot
// be represented by the on-disk format and would be treated as corruption
// on recovery, truncating every record written after them.
//
// Concurrent Appends are group-committed: each encodes its record under the
// lock, and one caller writes the accumulated batch with a single Write
// outside the lock. Append still returns only after its record is written.
func (w *WAL) Append(payload []byte) (Offset, error) {
	if len(payload) == 0 {
		return Offset{}, ErrEmptyPayload
	}
	if len(payload) > maxRecordBytes {
		return Offset{}, ErrPayloadTooLarge
	}
	if w.closed.Load() {
		return Offset{}, ErrClosed
	}
	if w.opts.maxBytes > 0 && w.totalSz.Load() > w.opts.maxBytes {
		return Offset{}, ErrFull
	}

	crc := crc32.ChecksumIEEE(payload)

	w.wmu.Lock()
	defer w.wmu.Unlock()

	// Re-check under the lock: Close stores w.closed = true while holding
	// this mutex before it closes the active file.
	if w.closed.Load() {
		return Offset{}, ErrClosed
	}
	if w.active == nil || w.active.file == nil {
		return Offset{}, ErrClosed
	}

	b, gen := w.cur, w.curGen
	idx := b.add(payload, crc)
	for w.doneGen < gen {
		if !w.flushing {
			w.flushLocked()
			continue
		}
		w.flushed.Wait()
	}

	off, err := b.offs[idx], b.errs[idx]
	if b.refs--; b.refs == 0 {
		putAppendBatch(b)
	}
	return off, err
}

// flushLocked writes the accumulating batch and publishes its results.
// Caller must hold w.wmu; it is released around each Write. w.mu is taken
// only to roll a segment or roll back a failed write, so ReadPending and Ack
// never delay a commit.
func (w *WAL) flushLocked() {
	b := w.cur
	gen := w.curGen
	w.cur = getAppendBatch()
	w.curGen++
	w.flushing = true

	for i := 0; i < b.len(); {
		if w.closed.Load() || w.active == nil || w.active.file == nil {
			b.fail(i, b.len(), ErrClosed)
			break
		}
		// Roll segment if needed, with the same rule a single Append applies.
		seg := w.active
		if seg.size.Load()+b.size(i) > w.opts.maxSegmentBytes && seg.records.Load() > 0 {
			w.mu.Lock()
			err := w.sealActiveLocked()
			if err == nil {
				err = w.openNewActiveLocked()
			}
			w.mu.Unlock()
			if err != nil {
				b.fail(i, b.len(), err)
				break
			}
			continue
		}
		// Take the longest run of records that fits the segment.
		j, size, records := i, seg.size.Load(), seg.records.Load()
		for j < b.len() && (size+b.size(j) <= w.opts.maxSegmentBytes || records == 0) {
			size += b.size(j)
			records++
			j++
		}

		f := seg.file
		w.wmu.Unlock()
		n, err := f.Write(b.buf[b.starts[i]:b.starts[j]])
		w.wmu.Lock()

		if err != nil {
			w.mu.Lock()
			err = w.rollbackLocked(seg, n, err)
			w.mu.Unlock()
			b.fail(i, j, err)
			i = j
			continue
		}
		pos := seg.size.Load()
		for k := i; k < j; k++ {
			b.offs[k] = Offset{SegmentID: seg.id, Position: pos}
			pos += b.size(k)
		}
		written := int64(j - i)
		seg.records.Add(written)
		seg.unacked.Add(written)
		seg.size.Store(pos)
		w.totalSz.Add(int64(b.starts[j] - b.starts[i]))
		i = j
	}

	w.doneGen = gen
	w.flushing = false
	w.flushed.Broadcast()
}

// rollbackLocked undoes a partial write of n bytes and returns the error to
// report. The io.Writer contract allows Write to return (n < len(p), err),
// and os.File honors it for errors such as ENOSPC or EFBIG hit mid-buffer.
// Those bytes advance the file position, so without a roll-back seg.size
// diverges from the on-disk length and later writes land at the wrong
// offset. If the roll-back itself fails the segment is in an indeterminate
// state: its file handle is retired so every later Append returns
// ErrClosed, and the next Open recovers the consistent prefix.
func (w *WAL) rollbackLocked(seg *segmentRef, n int, err error) error {
	if n > 0 && seg.file != nil {
		if truncErr := seg.file.Truncate(seg.size.Load()); truncErr != nil {
			_ = seg.file.Close()
			seg.file = nil
			return coreerrs.Wrapf(err, "wal: write record (truncate rollback failed: %v)", truncErr)
		}
		if _, seekErr := seg.file.Seek(seg.size.Load(), io.SeekStart); seekErr != nil {
			_ = seg.file.Close()
			seg.file = nil
			return coreerrs.Wrapf(err, "wal: write record (seek rollback failed: %v)", seekErr)
		}
	}
	return coreerrs.Wrap(err, "wal: write record")
}

// sealActiveLocked closes the active segment file and moves it to the
// sealed list. Caller must hold w.wmu and w.mu.
func (w *WAL) sealActiveLocked() error {
	seg := w.active
	if seg == nil {
		return nil
	}
	if seg.file != nil {
		if err := seg.file.Sync(); err != nil {
			return coreerrs.Wrap(err, "wal: final sync")
		}
		if err := seg.file.Close(); err != nil {
			return coreerrs.Wrap(err, "wal: close")
		}
		seg.file = nil
	}
	if seg.unacked.Load() > 0 {
		w.sealed = append(w.sealed, seg)
	} else {
		// Empty or fully acked: no later Ack would reach it, so remove it now
		// rather than replay acked records on the next Open.
		_ = os.Remove(seg.path)
		w.totalSz.Add(-seg.size.Load())
	}
	w.active = nil
	return nil
}

// Ack marks one record at the given offset as acknowledged by the consumer.
// Sealed segments whose records are all acked are deleted from disk.
// Calling Ack with a zero offset is a no-op.
func (w *WAL) Ack(offset Offset) {
	if offset.IsZero() || w.closed.Load() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.active != nil && w.active.id == offset.SegmentID {
		w.active.ack(offset.Position)
		return
	}
	for i, s := range w.sealed {
		if s.id == offset.SegmentID {
			s.ack(offset.Position)
			if s.unacked.Load() <= 0 {
				_ = os.Remove(s.path)
				w.totalSz.Add(-s.size.Load())
				w.sealed = slices.Delete(w.sealed, i, i+1)
			}
			return
		}
	}
}

func (w *WAL) fsyncLoop() {
	ticker := time.NewTicker(w.opts.fsyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.fsyncDone:
			w.fsyncOnce()
			return
		case <-ticker.C:
			w.fsyncOnce()
		}
	}
}

func (w *WAL) fsyncOnce() {
	w.mu.Lock()
	var (
		f  *os.File
		id uint64
	)
	if w.active != nil {
		f = w.active.file
		id = w.active.id
	}
	w.mu.Unlock()
	if f == nil {
		return
	}
	if err := f.Sync(); err != nil {
		// The active segment may have been rotated between capturing f and
		// calling Sync: Append/sealActiveLocked syncs and closes the old
		// file under w.mu, so f.Sync then returns os.ErrClosed. Because the
		// seal path already fsynced that segment, the data is durable and
		// the error is benign — do not report it as a fault.
		w.mu.Lock()
		rotated := w.active == nil || w.active.file != f
		w.mu.Unlock()
		if rotated && errors.Is(err, os.ErrClosed) {
			return
		}
		// Stash the latest failure so callers can observe via
		// [WAL.Faulted]. Previously the error was logged and dropped —
		// a durability primitive that loses an fsync should at minimum
		// expose the state to consumers who actually care about the
		// guarantee.
		w.lastFsyncErr.Store(&err)
		w.opts.logger.Error("wal: background fsync failed",
			slog.Uint64("segment_id", id),
			slog.Any("error", err),
		)
		return
	}
	// Success — clear any previous error so transient failures don't
	// stick as a "permanently faulted" signal.
	w.lastFsyncErr.Store(nil)
}

// Faulted reports the last background-fsync error and nil-if-healthy.
// Returns nil when no fsync has failed since the last successful one;
// otherwise returns the captured error so consumers can fail-stop,
// page operators, or surface a metric. The check is lock-free and
// cheap to poll.
func (w *WAL) Faulted() error {
	if p := w.lastFsyncErr.Load(); p != nil {
		return *p
	}
	return nil
}

// Sync forces an immediate fsync on the active segment.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == nil || w.active.file == nil {
		return nil
	}
	return w.active.file.Sync()
}

// Close performs a final fsync, stops the background goroutine, and closes
// all open files. The active segment is removed from disk if every record
// in it has already been acked. Sealed-but-unacked segments remain on disk
// and will be recovered next time the WAL is opened.
//
// Any error encountered during the final Sync or Close of the active
// segment is aggregated via [errors.Join] and returned — a durability
// primitive must not silently drop the final flush result. Calling
// Close more than once is a no-op and returns nil.
func (w *WAL) Close() error {
	// Single-winner guard: exactly one caller proceeds to stop the fsync
	// goroutine and close files; concurrent and repeated calls return nil
	// immediately. Guarding with w.closed under the mutex is not enough —
	// it is only set further down, so two racing callers could both pass
	// the check and both close(w.fsyncDone), panicking. The closed flag
	// is still set late, under w.mu, so in-flight Acks from consumer
	// goroutines that finish concurrently still take effect.
	if !w.closing.CompareAndSwap(false, true) {
		return nil
	}

	close(w.fsyncDone)
	w.fsyncWG.Wait()

	w.wmu.Lock()
	defer w.wmu.Unlock()
	w.mu.Lock()
	w.closed.Store(true)
	w.mu.Unlock()
	// Let an in-flight group commit finish before closing its file; queued
	// Appends then fail with ErrClosed.
	for w.flushing {
		w.flushed.Wait()
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	var errs []error
	if w.active != nil {
		if w.active.file != nil {
			if err := w.active.file.Sync(); err != nil {
				errs = append(errs, coreerrs.Wrap(err, "wal: final sync"))
			}
			if err := w.active.file.Close(); err != nil {
				errs = append(errs, coreerrs.Wrap(err, "wal: final close"))
			}
			w.active.file = nil
		}
		// If everything in the active segment was already acked, drop the
		// file so a clean shutdown leaves the WAL directory empty.
		if w.active.unacked.Load() <= 0 {
			if err := os.Remove(w.active.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, coreerrs.Wrapf(err, "wal: remove %s", w.active.path))
			}
			w.totalSz.Add(-w.active.size.Load())
			w.active = nil
		}
	}
	return errors.Join(errs...)
}

// Stats returns current WAL statistics.
func (w *WAL) Stats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := Stats{TotalBytes: w.totalSz.Load()}
	for _, seg := range w.sealed {
		s.Pending += seg.unacked.Load()
	}
	if w.active != nil {
		s.Pending += w.active.unacked.Load()
		s.Segments = 1 + len(w.sealed)
	} else {
		s.Segments = len(w.sealed)
	}
	return s
}
