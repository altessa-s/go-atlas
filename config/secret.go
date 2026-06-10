// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"github.com/altessa-s/go-atlas/core/types/redacted"
)

// Secret is the in-package alias for
// [github.com/altessa-s/go-atlas/core/types/redacted.RedactedString].
//
// It exists so that configuration structs declared in this package can
// keep a short, locally meaningful spelling (`Password Secret` reads
// naturally next to other config fields). Callers outside this package
// can use either Secret or redacted.RedactedString — they are the same
// type. New code unrelated to config loading should prefer
// redacted.RedactedString to avoid an unnecessary config dependency.
type Secret = redacted.RedactedString
