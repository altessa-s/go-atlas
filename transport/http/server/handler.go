// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import "github.com/altessa-s/go-atlas/transport/http/server/writer"

// RouteRegistrar registers routes with ReadWriter support.
// This minimal interface decouples handlers from the concrete Server type,
// making handlers easier to test and more flexible.
type RouteRegistrar interface {
	// Handle registers a HandlerFunc for the given pattern.
	// Returns Route for further configuration (Methods, PathPrefix, etc.)
	Handle(pattern string, handler HandlerFunc) Route
}

// Handler registers HTTP routes with a [RouteRegistrar].
// Pass implementations to [Server.RegisterHandlers] before calling [Server.Start].
//
// Example:
//
//	type MyHandler struct{}
//
//	func (h *MyHandler) Register(r http.RouteRegistrar, stop <-chan struct{}) {
//	    r.Handle("/users", h.listUsers).Methods("GET")
//	    r.Handle("/users", h.createUser).Methods("POST")
//	    r.Handle("/users/{id}", h.getUser).Methods("GET")
//	}
//
//	func (h *MyHandler) listUsers(rw writer.ReadWriter) {
//	    // handler implementation
//	}
type Handler interface {
	// Register registers routes with the registrar.
	// r provides Handle() for registering HandlerFunc routes.
	// stop channel signals server shutdown for cleanup.
	Register(r RouteRegistrar, stop <-chan struct{})
}

// HandlerFunc is a function type for handlers that use [writer.ReadWriter].
// Register it via [Server.Handle] or [RouteRegistrar.Handle].
//
// Example:
//
//	func createUser(rw writer.ReadWriter) {
//	    var input CreateUserRequest
//	    if err := rw.Read(&input); err != nil {
//	        rw.WriteError(err, http.StatusBadRequest)
//	        return
//	    }
//	    user := createUser(input)
//	    rw.Write(user)
//	}
//
//	server.Handle("/users", createUser).Methods("POST")
type HandlerFunc func(rw writer.ReadWriter)
