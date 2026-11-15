// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// Mount mounts handler at /metrics/ on r and returns the subrouter. The
// handler comes from the metrics backend, for example
// promadapter.DefaultHandler() or (*promadapter.Adapter).Handler(), so this
// package depends on no metrics library.
func Mount(r router.Router, handler http.Handler) router.Router {
	metricsRouter := r.PathPrefix("/metrics").Subrouter()
	metricsRouter.Handle("/", handler)

	return metricsRouter
}
