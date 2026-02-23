// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
)

// Frame represents a single call-stack entry with the source file path,
// line number, and short function name (package.Function, not the full
// import path).
type Frame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"func"`
}

// Frames is an ordered slice of [Frame] values from innermost to outermost
// caller. It implements [fmt.Stringer] and [json.Marshaler].
type Frames []Frame

// String renders all frames as "file:line function" lines separated by
// newlines, suitable for human-readable log output.
func (f Frames) String() string {
	if len(f) == 0 {
		return ""
	}

	var b strings.Builder
	b.Grow(len(f) * estimatedFrameSize) // Estimate bytes per frame
	for i, frame := range f {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(frame.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(frame.Line))
		b.WriteByte(' ')
		b.WriteString(frame.Function)
	}
	return b.String()
}

// MarshalJSON implements json.Marshaler for Frames.
func (f Frames) MarshalJSON() ([]byte, error) {
	if len(f) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]Frame(f))
}

// stackBufferSize is the buffer size for capturing stack frames.
const stackBufferSize = 32

// estimatedFrameSize is the estimated size of a single stack frame in bytes.
const estimatedFrameSize = 64

// StackSkipOffset is the number of stack frames to skip beyond the caller's skip value.
// This accounts for runtime.Callers and StackTrace function frames themselves.
// When calling StackTrace(0), the caller's frame will be the first in the result.
const StackSkipOffset = 2

// StackTrace captures the current goroutine's call stack. With skip=0 the
// first frame is the caller of StackTrace. At most [stackBufferSize] (32)
// frames are captured.
func StackTrace(skip int) Frames {
	b := make([]uintptr, stackBufferSize)
	l := runtime.Callers(skip+StackSkipOffset, b[:]) //nolint:gocritic

	frames := make([]Frame, 0, l)
	runtimeFrames := runtime.CallersFrames(b[:l])

	for {
		runtimeFrame, more := runtimeFrames.Next()
		frames = append(frames, Frame{
			File:     runtimeFrame.File,
			Line:     runtimeFrame.Line,
			Function: shortname(runtimeFrame.Function),
		})
		if !more {
			break
		}
	}

	return frames
}

// shortname extracts the short function name from a fully qualified name.
func shortname(name string) string {
	i := strings.LastIndexByte(name, '/')
	return name[i+1:]
}
