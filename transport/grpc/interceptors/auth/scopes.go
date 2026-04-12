// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"iter"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// Scope represents an authorization scope string used for method-level access control.
// Scopes are typically strings that define permissions or access levels required
// to invoke specific gRPC methods. Common examples include "read", "write", "admin",
// or more specific scopes like "user:read", "billing:write".
type Scope = string

// ScopeNone represents no authorization scope requirement.
// Methods associated with ScopeNone can be accessed without specific scope validation.
// This is useful for public endpoints or methods that only require authentication
// but not specific authorization scopes.
var ScopeNone = ""

// ScopeRegistry manages the mapping between gRPC method names and their required authorization scopes.
// It provides a centralized way to define and retrieve scope requirements for different methods
// in a gRPC service.
//
// Method names should follow the full gRPC format: "/package.Service/Method"
// For example: "/user.UserService/GetUser" or "/billing.BillingService/CreateInvoice"
//
// Security consideration: The ScopeRegistry should be populated during service initialization
// and frozen via [ScopeRegistry.Freeze] before handling requests. After freezing, the registry
// is safe for concurrent reads without synchronization.
//
// Performance note: Scope lookups are O(1) operations.
type ScopeRegistry struct {
	// building is the mutable map used during the registration phase.
	building map[string]Scope
	// frozen is the immutable map used after Freeze() is called. Nil until frozen.
	frozen *coremaps.ImmutableMap[string, Scope]
}

// NewScopeRegistry creates a new empty ScopeRegistry.
// The returned registry is ready to use and can be populated using Register or RegisterMethods.
// Call [ScopeRegistry.Freeze] after registration is complete to make the registry immutable
// and safe for concurrent reads.
//
// Example:
//
//	registry := auth.NewScopeRegistry()
//	registry.Register("/user.UserService/GetUser", "user:read")
//	registry.Register("/user.UserService/UpdateUser", "user:write")
//	registry.Freeze()
func NewScopeRegistry() *ScopeRegistry {
	return &ScopeRegistry{
		building: make(map[string]Scope),
	}
}

// Register associates a gRPC method with a required authorization scope.
// If the method was previously registered, its scope will be updated to the new value.
// Panics if called after [ScopeRegistry.Freeze].
//
// Parameters:
//   - methodName: Full gRPC method name in format "/package.Service/Method"
//   - scope: Required authorization scope for the method, or ScopeNone for no scope requirement
//
// Example:
//
//	registry.Register("/user.UserService/GetUser", "user:read")
//	registry.Register("/user.UserService/DeleteUser", "user:admin")
//	registry.Register("/health.HealthService/Check", auth.ScopeNone)
//
// Security note: Unregistered methods are denied by default — Scope() returns (_, false)
// for methods not in the registry. Use ScopeNone to explicitly mark methods as public.
func (r *ScopeRegistry) Register(methodName string, scope Scope) {
	if r.frozen != nil {
		panic("auth: Register called on frozen ScopeRegistry")
	}
	r.building[methodName] = scope
}

// RegisterMethods associates multiple gRPC methods with the same required authorization scope.
// This is a convenience method for bulk registration when multiple methods share the same scope requirement.
// If any of the methods were previously registered, their scopes will be updated to the new value.
// Panics if called after [ScopeRegistry.Freeze].
//
// Parameters:
//   - scope: Required authorization scope for all the specified methods
//   - methodNames: Variable number of full gRPC method names in format "/package.Service/Method"
//
// Example:
//
//	// Register multiple read-only methods with the same scope
//	registry.RegisterMethods("user:read",
//		"/user.UserService/GetUser",
//		"/user.UserService/ListUsers",
//		"/user.UserService/GetUserProfile",
//	)
//
//	// Register admin methods
//	registry.RegisterMethods("admin",
//		"/user.UserService/DeleteUser",
//		"/user.UserService/BanUser",
//	)
//
// Performance note: This method is more efficient than multiple Register calls
// when registering many methods with the same scope.
func (r *ScopeRegistry) RegisterMethods(scope Scope, methodNames ...string) {
	if r.frozen != nil {
		panic("auth: RegisterMethods called on frozen ScopeRegistry")
	}
	for _, methodName := range methodNames {
		r.building[methodName] = scope
	}
}

// Freeze converts the internal map to an immutable representation.
// After this call, the registry is safe for concurrent reads without synchronization.
// Subsequent calls to [ScopeRegistry.Register] or [ScopeRegistry.RegisterMethods] will panic.
// Freeze is idempotent — calling it multiple times is safe.
func (r *ScopeRegistry) Freeze() {
	if r.frozen != nil {
		return
	}
	r.frozen = coremaps.NewImmutableMap(r.building)
	r.building = nil
}

// Scope retrieves the required authorization scope for a specific gRPC method.
// Returns the scope and true if the method is registered, or ("", false) if not.
//
// Callers MUST check the boolean return value. Unregistered methods should be
// denied by default to prevent accidentally exposing new endpoints without
// proper authorization.
//
// Parameters:
//   - methodName: Full gRPC method name in format "/package.Service/Method"
//
// Returns:
//   - scope: The required scope for the method (may be ScopeNone for public endpoints)
//   - ok: true if the method is registered, false otherwise
//
// Example:
//
//	scope, ok := registry.Scope("/user.UserService/GetUser")
//	if !ok {
//		// Method is not registered — deny by default
//		return nil, status.Error(codes.PermissionDenied, "method not registered in scope registry")
//	}
//	if scope != auth.ScopeNone {
//		// Validate that the caller has the required scope
//		fmt.Printf("Method requires scope: %s\n", scope)
//	}
//
// Security consideration: Always deny access for unregistered methods (ok == false).
// Methods explicitly registered with ScopeNone are intentionally public.
//
// Performance note: This is an O(1) operation.
func (r *ScopeRegistry) Scope(methodName string) (Scope, bool) {
	if r.frozen != nil {
		return r.frozen.Get(methodName)
	}
	scope, ok := r.building[methodName]
	return scope, ok
}

// AllScopes returns an iterator over all registered method-to-scope mappings.
// The iterator is safe to use concurrently after [ScopeRegistry.Freeze].
//
// Example:
//
//	for methodName, scope := range registry.AllScopes() {
//		if scope == auth.ScopeNone {
//			fmt.Printf("%s: no scope required\n", methodName)
//		} else {
//			fmt.Printf("%s: requires scope '%s'\n", methodName, scope)
//		}
//	}
//
// Use cases:
//   - Debugging and auditing scope configurations
//   - Generating documentation of API permissions
//   - Implementing scope validation logic
//   - Creating administrative interfaces
func (r *ScopeRegistry) AllScopes() iter.Seq2[string, Scope] {
	if r.frozen != nil {
		return r.frozen.All()
	}
	return func(yield func(string, Scope) bool) {
		for k, v := range r.building {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Len returns the number of registered methods.
func (r *ScopeRegistry) Len() int {
	if r.frozen != nil {
		return r.frozen.Len()
	}
	return len(r.building)
}
