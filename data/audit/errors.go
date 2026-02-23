// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "errors"

// ErrAuditorAlreadyStarted is returned by [Auditor.Start] when the auditor
// has already been started.
var ErrAuditorAlreadyStarted = errors.New("auditor already started")

// ErrBufferFull is returned when the internal event channel is full and the
// event is dropped. See [Auditor.Emit].
var ErrBufferFull = errors.New("event buffer full, event dropped")

// ErrNilStorage is returned by [New] when a nil [Storage] is provided.
var ErrNilStorage = errors.New("storage is required for auditor")
