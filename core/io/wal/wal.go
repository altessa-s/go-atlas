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

	dirMode  os.FileMode = 0o755 // WAL directory permissions.
	fileMode os.FileMode = 0o644 // WAL segment file permissions.
)

// Sentinel errors.
var (
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
}

// segmentRef tracks a single segment file and its outstanding (unacked) record count.
type segmentRef struct {
	id      uint64
	path    string
	file    *os.File // nil for sealed segments
	size    int64
	records int64 // total records ever appended to this segment
	unacked int64 // records not yet acked by consumer
	sealed  bool
}

// WAL is a segmented append-only log providing crash-safe durability for
// opaque byte payloads. Records are written length-prefixed with a CRC32;
// a background goroutine fsyncs the active segment on a fixed interval.
//
// WAL is safe for concurrent calls to Append, Ack, Sync, Stats, and Close.
type WAL struct {
	dir  string
	opts *options

	mu      sync.Mutex
	active  *segmentRef   // current writable segment
	sealed  []*segmentRef // sealed segments waiting for full ack
	nextID  uint64
	scratch []byte // reusable encode buffer; guarded by mu
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
// dir is created with 0755 if it does not exist. Additional tunables are
// supplied via [Option] values (see [WithMaxSegmentBytes], [WithMaxBytes]
// and [WithFsyncInterval]).
//
// A background fsync goroutine is started before Open returns.
func Open(dir string, opts ...Option) (*WAL, []Record, error) {
	if dir == "" {
		return nil, nil, errors.New("wal: dir is required")
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, nil, coreerrs.Wrap(err, "wal: mkdir")
	}

	w := &WAL{
		dir:       dir,
		opts:      newOptions(opts...),
		fsyncDone: make(chan struct{}),
		nextID:    1, // 0 is reserved for "no record" so the zero Offset is unambiguous.
	}

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
		ref := &segmentRef{
			id:      sf.id,
			path:    sf.path,
			size:    validSize,
			records: int64(len(recs)),
			unacked: int64(len(recs)),
			sealed:  true,
		}
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
// hold w.mu when called from Open (single-threaded), but must hold it
// when called during a roll inside Append.
func (w *WAL) openNewActiveLocked() error {
	id := w.nextID
	w.nextID++
	name := fmt.Sprintf("%020d%s", id, fileSuffix)
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
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
// Header and payload are coalesced into a single write under the lock,
// which halves the syscall count relative to a naive two-write encoding.
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

	total := int64(recordHeaderSize + len(payload))
	crc := crc32.ChecksumIEEE(payload)

	w.mu.Lock()
	defer w.mu.Unlock()

	// Re-check under the lock. Close stores w.closed = true while holding
	// this same mutex and then nils w.active.file; an Appender that slipped
	// past the early check above but lost the lock race to Close would
	// otherwise reach seg.file.Write on a nil *os.File (which the stdlib
	// surfaces as os.ErrInvalid, "invalid argument") or, worse, write
	// through a just-closed fd.
	if w.closed.Load() {
		return Offset{}, ErrClosed
	}
	if w.active == nil || w.active.file == nil {
		return Offset{}, ErrClosed
	}

	// Roll segment if needed.
	if w.active.size+total > w.opts.maxSegmentBytes && w.active.records > 0 {
		if err := w.sealActiveLocked(); err != nil {
			return Offset{}, err
		}
		if err := w.openNewActiveLocked(); err != nil {
			return Offset{}, err
		}
	}

	// Encode [len][crc][payload] into the reusable scratch buffer held
	// under the mutex, then issue a single Write. This avoids the per-
	// record allocation of a fresh buffer and halves the syscall count
	// compared to writing header and payload separately.
	need := int(total)
	if cap(w.scratch) < need {
		w.scratch = make([]byte, need)
	} else {
		w.scratch = w.scratch[:need]
	}
	binary.LittleEndian.PutUint32(w.scratch[0:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(w.scratch[4:8], crc)
	copy(w.scratch[recordHeaderSize:], payload)

	seg := w.active
	pos := seg.size
	if n, err := seg.file.Write(w.scratch); err != nil {
		// Partial-write roll-back. The io.Writer contract allows Write
		// to return (n < len(p), err), and os.File honors it for errors
		// such as ENOSPC or EFBIG that are hit mid-buffer. Those n bytes
		// are already in the kernel page cache and advance the file's
		// position cursor, so without a roll-back seg.size diverges
		// from the on-disk length and every subsequent Append in this
		// process writes at the wrong offset — corrupting the segment
		// in-place (even though the next Open would cleanly truncate
		// the torn trailer via the CRC check in readSegment).
		//
		// Best-effort: truncate the file back to seg.size and rewind
		// the position. If either rollback syscall itself fails the
		// segment is in an indeterminate state — retire its file
		// handle so the re-checks above turn every future Append into
		// ErrClosed. The on-disk prefix remains consistent up to the
		// last successfully acked record, and the next Open recovers
		// from there.
		if n > 0 {
			if truncErr := seg.file.Truncate(seg.size); truncErr != nil {
				_ = seg.file.Close()
				seg.file = nil
				return Offset{}, coreerrs.Wrapf(err,
					"wal: write record (truncate rollback failed: %v)", truncErr)
			}
			if _, seekErr := seg.file.Seek(seg.size, io.SeekStart); seekErr != nil {
				_ = seg.file.Close()
				seg.file = nil
				return Offset{}, coreerrs.Wrapf(err,
					"wal: write record (seek rollback failed: %v)", seekErr)
			}
		}
		return Offset{}, coreerrs.Wrap(err, "wal: write record")
	}
	seg.size += total
	seg.records++
	seg.unacked++
	w.totalSz.Add(total)

	return Offset{SegmentID: seg.id, Position: pos}, nil
}

// sealActiveLocked closes the active segment file and moves it to the
// sealed list. Caller must hold w.mu.
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
	seg.sealed = true
	if seg.records > 0 {
		w.sealed = append(w.sealed, seg)
	} else {
		// Empty segment: remove.
		_ = os.Remove(seg.path)
		w.totalSz.Add(-seg.size)
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
		if w.active.unacked > 0 {
			w.active.unacked--
		}
		return
	}
	for i, s := range w.sealed {
		if s.id == offset.SegmentID {
			if s.unacked > 0 {
				s.unacked--
			}
			if s.unacked <= 0 {
				_ = os.Remove(s.path)
				w.totalSz.Add(-s.size)
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

	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed.Store(true)

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
		if w.active.unacked <= 0 {
			if err := os.Remove(w.active.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, coreerrs.Wrapf(err, "wal: remove %s", w.active.path))
			}
			w.totalSz.Add(-w.active.size)
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
	if w.active != nil {
		s.Segments = 1 + len(w.sealed)
	} else {
		s.Segments = len(w.sealed)
	}
	return s
}
