// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "errors"

// ErrAuditorAlreadyStarted is returned by [Auditor.Start] when the auditor
// has already been started.
var ErrAuditorAlreadyStarted = errors.New("auditor already started")

// ErrNilDispatcher is returned by [New] when a nil [Dispatcher] is provided.
var ErrNilDispatcher = errors.New("dispatcher is required for auditor")
