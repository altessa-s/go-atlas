// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package aipmeter adapts a go-atlas [github.com/altessa-s/go-atlas/observability/metrics.Meter]
// to the metrics API of go-aip ([github.com/altessa-s/go-aip/metrics.Meter]),
// so the go-aip filter, order and projection parsers and the backend
// translators record into the application's collector:
//
//	p, err := parser.New(parser.WithCollector(aipmeter.New(collector)))
//
// The parsers record parse_duration_seconds and parse_errors_total under the
// subsystems filter, orderby and projection; a translator records
// translate_duration_seconds under filter_<backend>, orderby_<backend> or
// projection_<backend>.
package aipmeter
