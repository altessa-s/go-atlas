// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errs

import (
	"errors"
)

// ErrProviderStopped is returned when operating on a stopped or unstarted provider.
var ErrProviderStopped = errors.New("provider stopped")
