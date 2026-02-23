// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

func TestAuthError_Error(t *testing.T) {
	t.Run("without wrapped", func(t *testing.T) {
		err := auth.NewAuthError("approle", "invalid role ID", 401, nil)
		expected := "auth method approle failed: invalid role ID (code: 401)"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("with wrapped", func(t *testing.T) {
		wrapped := errors.New("connection refused")
		err := auth.NewAuthError("approle", "network error", 500, wrapped)
		expected := "auth method approle failed: network error (code: 500): connection refused"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})
}

func TestAuthError_Unwrap(t *testing.T) {
	wrapped := errors.New("underlying error")
	err := auth.NewAuthError("approle", "test", 401, wrapped)
	if err.Unwrap() != wrapped {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), wrapped)
	}
}

func TestAuthError_Is(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		target error
		want   bool
	}{
		{"401 matches ErrUnauthorized", 401, auth.ErrUnauthorized, true},
		{"401 matches ErrInvalidCredentials", 401, auth.ErrInvalidCredentials, true},
		{"403 matches ErrUnauthorized", 403, auth.ErrUnauthorized, true},
		{"403 matches ErrPermissionDenied", 403, auth.ErrPermissionDenied, true},
		{"500 does not match ErrUnauthorized", 500, auth.ErrUnauthorized, false},
		{"500 does not match ErrInvalidCredentials", 500, auth.ErrInvalidCredentials, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auth.NewAuthError("test", "test error", tt.code, nil)
			if got := errors.Is(err, tt.target); got != tt.want {
				t.Errorf("Is(%v) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestNewAuthError(t *testing.T) {
	wrapped := errors.New("wrapped error")
	err := auth.NewAuthError("approle", "invalid credentials", 401, wrapped)

	if err.Method != "approle" {
		t.Errorf("Method = %q, want %q", err.Method, "approle")
	}
	if err.Reason != "invalid credentials" {
		t.Errorf("Reason = %q, want %q", err.Reason, "invalid credentials")
	}
	if err.Code != 401 {
		t.Errorf("Code = %d, want 401", err.Code)
	}
	if err.Wrapped != wrapped {
		t.Errorf("Wrapped = %v, want %v", err.Wrapped, wrapped)
	}
}

func TestWrapAuthError_Nil(t *testing.T) {
	if auth.WrapAuthError("approle", nil) != nil {
		t.Error("WrapAuthError(nil) should return nil")
	}
}

func TestWrapAuthError_AlreadyAuthError(t *testing.T) {
	original := auth.NewAuthError("approle", "test", 401, nil)
	wrapped := auth.WrapAuthError("userpass", original)
	if wrapped != original {
		t.Error("WrapAuthError should not double-wrap AuthError")
	}
}

func TestWrapAuthError_GenericError(t *testing.T) {
	original := errors.New("generic error")
	wrapped := auth.WrapAuthError("approle", original)

	authErr, ok := wrapped.(*auth.AuthError)
	if !ok {
		t.Fatalf("expected *AuthError, got %T", wrapped)
	}
	if authErr.Method != "approle" {
		t.Errorf("Method = %q, want %q", authErr.Method, "approle")
	}
	if authErr.Code != 0 {
		t.Errorf("Code = %d, want 0", authErr.Code)
	}
	if !errors.Is(wrapped, original) {
		t.Error("should unwrap to original")
	}
}

func TestIsAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"ErrUnauthorized", auth.ErrUnauthorized, true},
		{"ErrInvalidCredentials", auth.ErrInvalidCredentials, true},
		{"ErrPermissionDenied", auth.ErrPermissionDenied, true},
		{"generic error", errors.New("some error"), false},
		{"AuthError 401", auth.NewAuthError("test", "unauthorized", 401, nil), true},
		{"AuthError 500", auth.NewAuthError("test", "server error", 500, nil), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.IsAuthenticationError(tt.err); got != tt.want {
				t.Errorf("IsAuthenticationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"auth error 401", auth.NewAuthError("test", "unauthorized", 401, nil), false},
		{"generic error", errors.New("network error"), true},
		{"vault 500", &vaultApi.ResponseError{StatusCode: 500}, true},
		{"vault 503", &vaultApi.ResponseError{StatusCode: 503}, true},
		{"vault 429", &vaultApi.ResponseError{StatusCode: http.StatusTooManyRequests}, true},
		{"vault 400", &vaultApi.ResponseError{StatusCode: 400}, false},
		{"vault 401", &vaultApi.ResponseError{StatusCode: 401}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.IsRetryableError(tt.err); got != tt.want {
				t.Errorf("IsRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWrapAuthError_VaultResponseError(t *testing.T) {
	vaultErr := &vaultApi.ResponseError{
		StatusCode: 401,
		Errors:     []string{"invalid secret id"},
	}
	wrapped := auth.WrapAuthError("approle", vaultErr)

	authErr, ok := wrapped.(*auth.AuthError)
	if !ok {
		t.Fatalf("expected *AuthError, got %T", wrapped)
	}
	if authErr.Code != 401 {
		t.Errorf("Code = %d, want 401", authErr.Code)
	}
	if authErr.Reason != "invalid secret id" {
		t.Errorf("Reason = %q, want %q", authErr.Reason, "invalid secret id")
	}
}

func TestWrapAuthError_VaultResponseError_NoErrors(t *testing.T) {
	vaultErr := &vaultApi.ResponseError{
		StatusCode: 500,
		Errors:     nil,
	}
	wrapped := auth.WrapAuthError("approle", vaultErr)

	authErr, ok := wrapped.(*auth.AuthError)
	if !ok {
		t.Fatalf("expected *AuthError, got %T", wrapped)
	}
	if authErr.Reason != "authentication failed" {
		t.Errorf("Reason = %q, want %q", authErr.Reason, "authentication failed")
	}
}

func TestWrapAuthError_VaultResponseError_UnknownMessage(t *testing.T) {
	vaultErr := &vaultApi.ResponseError{
		StatusCode: 500,
		Errors:     []string{"some obscure internal error with secrets"},
	}
	wrapped := auth.WrapAuthError("approle", vaultErr)

	authErr, ok := wrapped.(*auth.AuthError)
	if !ok {
		t.Fatalf("expected *AuthError, got %T", wrapped)
	}
	// Unknown messages should be sanitized to generic
	if authErr.Reason != "authentication failed" {
		t.Errorf("Reason = %q, want %q", authErr.Reason, "authentication failed")
	}
}

func TestWrapAuthError_VaultResponseError_KnownMessages(t *testing.T) {
	knownMessages := []struct {
		input string
		want  string
	}{
		{"invalid secret id", "invalid secret id"},
		{"invalid role id", "invalid role id"},
		{"permission denied", "permission denied"},
		{"unauthorized", "unauthorized"},
		{"connection refused", "connection refused"},
		{"timeout", "timeout"},
		{"service unavailable", "service unavailable"},
	}

	for _, tt := range knownMessages {
		t.Run(tt.input, func(t *testing.T) {
			vaultErr := &vaultApi.ResponseError{
				StatusCode: 500,
				Errors:     []string{tt.input},
			}
			wrapped := auth.WrapAuthError("test", vaultErr)
			authErr := wrapped.(*auth.AuthError)
			if authErr.Reason != tt.want {
				t.Errorf("Reason = %q, want %q", authErr.Reason, tt.want)
			}
		})
	}
}

func TestIsAuthenticationError_VaultResponseErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"vault 401", &vaultApi.ResponseError{StatusCode: 401}, true},
		{"vault 403", &vaultApi.ResponseError{StatusCode: 403}, true},
		{"vault 500", &vaultApi.ResponseError{StatusCode: 500}, false},
		{"vault invalid secret id", &vaultApi.ResponseError{StatusCode: 200, Errors: []string{"invalid secret id"}}, true},
		{"vault invalid role ID", &vaultApi.ResponseError{StatusCode: 200, Errors: []string{"invalid role ID"}}, true},
		{"vault invalid username or password", &vaultApi.ResponseError{StatusCode: 200, Errors: []string{"invalid username or password"}}, true},
		{"vault generic message", &vaultApi.ResponseError{StatusCode: 200, Errors: []string{"something else"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.IsAuthenticationError(tt.err); got != tt.want {
				t.Errorf("IsAuthenticationError() = %v, want %v", got, tt.want)
			}
		})
	}
}
