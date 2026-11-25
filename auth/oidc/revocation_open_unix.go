//go:build unix

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "syscall"

const nonblockOpenFlag = syscall.O_NONBLOCK
