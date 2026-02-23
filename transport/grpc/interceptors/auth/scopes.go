// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"maps"
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
// in a gRPC service. The registry is thread-safe for reads but not for concurrent writes.
//
// Method names should follow the full gRPC format: "/package.Service/Method"
// For example: "/user.UserService/GetUser" or "/billing.BillingService/CreateInvoice"
//
// Security consideration: The ScopeRegistry should be populated during service initialization
// and not modified during runtime to ensure consistent authorization behavior.
//
// Performance note: Scope lookups are O(1) operations using an internal map.
type ScopeRegistry struct {
	// scopes maps full gRPC method names to their required authorization scopes
	scopes map[string]Scope
}

// NewScopeRegistry creates a new empty ScopeRegistry.
// The returned registry is ready to use and can be populated using Register or RegisterMethods.
//
// Example:
//
//	registry := auth.NewScopeRegistry()
//	registry.Register("/user.UserService/GetUser", "user:read")
//	registry.Register("/user.UserService/UpdateUser", "user:write")
func NewScopeRegistry() *ScopeRegistry {
	return &ScopeRegistry{
		scopes: make(map[string]Scope),
	}
}

// Register associates a gRPC method with a required authorization scope.
// If the method was previously registered, its scope will be updated to the new value.
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
	r.scopes[methodName] = scope
}

// RegisterMethods associates multiple gRPC methods with the same required authorization scope.
// This is a convenience method for bulk registration when multiple methods share the same scope requirement.
// If any of the methods were previously registered, their scopes will be updated to the new value.
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
	for _, methodName := range methodNames {
		r.scopes[methodName] = scope
	}
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
// Performance note: This is an O(1) operation using map lookup.
func (r *ScopeRegistry) Scope(methodName string) (Scope, bool) {
	scope, ok := r.scopes[methodName]
	return scope, ok
}

// AllScopes returns a copy of all registered method-to-scope mappings.
// The returned map is a defensive copy and can be safely modified without affecting the registry.
//
// Returns:
//   - A map containing all method names (keys) and their required scopes (values)
//
// Example:
//
//	allScopes := registry.AllScopes()
//	for methodName, scope := range allScopes {
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
//
// Performance note: This method creates a full copy of the internal map,
// so it should not be called frequently in hot paths. The copy operation is O(n)
// where n is the number of registered methods.
func (r *ScopeRegistry) AllScopes() map[string]Scope {
	result := make(map[string]Scope, len(r.scopes))
	maps.Copy(result, r.scopes)
	return result
}
