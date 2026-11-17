// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package purgeloop provides a lazy, permanently stoppable background purge
// loop and bounded batch draining shared by SQL storages. Background passes
// carry no request values or transactions; each drain step defaults to a
// 30-second timeout.
package purgeloop
