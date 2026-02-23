// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestMongoDirectionConnectRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		hosts   []string
		value   any
		when    bool
		wantErr bool
	}{
		{
			name:    "single host, bool true, valid",
			hosts:   []string{"mongodb://localhost:27017"},
			value:   true,
			when:    true,
			wantErr: false,
		},
		{
			name:    "multiple hosts, bool true, invalid",
			hosts:   []string{"mongodb://host1:27017", "mongodb://host2:27017"},
			value:   true,
			when:    true,
			wantErr: true,
		},
		{
			name:    "SRV host, bool true, invalid",
			hosts:   []string{"mongodb+srv://cluster.example.com"},
			value:   true,
			when:    true,
			wantErr: true,
		},
		{
			name:    "SRV host with mixed case, bool true, invalid",
			hosts:   []string{"MongoDB+SRV://cluster.example.com"},
			value:   true,
			when:    true,
			wantErr: true,
		},
		{
			name:    "single host non-SRV, bool true, valid",
			hosts:   []string{"mongodb://localhost:27017"},
			value:   true,
			when:    true,
			wantErr: false,
		},
		{
			name:    "condition false (When(false)), no validation",
			hosts:   []string{"mongodb://host1:27017", "mongodb://host2:27017"},
			value:   true,
			when:    false,
			wantErr: false,
		},
		{
			name:    "nil value, error",
			hosts:   []string{"mongodb://localhost:27017"},
			value:   (*bool)(nil),
			when:    true,
			wantErr: true,
		},
		{
			name:    "non-bool value (string), no error",
			hosts:   []string{"mongodb://host1:27017", "mongodb://host2:27017"},
			value:   "true",
			when:    true,
			wantErr: false,
		},
		{
			name:    "empty hosts, bool true, valid",
			hosts:   []string{},
			value:   true,
			when:    true,
			wantErr: false,
		},
		{
			name:    "bool false with multiple hosts, still checked",
			hosts:   []string{"mongodb://host1:27017", "mongodb://host2:27017"},
			value:   false,
			when:    true,
			wantErr: true,
		},
		{
			name:    "bool false with SRV host, still checked",
			hosts:   []string{"mongodb+srv://cluster.example.com"},
			value:   false,
			when:    true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := validators.MongoDirectionConnect(tt.hosts)
			if !tt.when {
				rule = rule.When(false)
			}
			err := rule.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("MongoDirectionConnectRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if err.Error() != validators.ErrDirectionConnectInvalid.Error() {
					t.Errorf("MongoDirectionConnectRule.Validate() error message = %v, want %v", err.Error(), validators.ErrDirectionConnectInvalid.Error())
				}
			}
		})
	}
}

func TestMongoDirectionConnectRule_When(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	rule := validators.MongoDirectionConnect(hosts).When(false)

	// Should not return error even with invalid configuration because condition is false
	err := rule.Validate(true)
	if err != nil {
		t.Errorf("MongoDirectionConnectRule.When(false) should skip validation, got error: %v", err)
	}

	// With condition true, should return error
	rule = validators.MongoDirectionConnect(hosts).When(true)
	err = rule.Validate(true)
	if err == nil {
		t.Error("MongoDirectionConnectRule.When(true) should validate and return error for multiple hosts")
	}
}

func TestMongoDirectionConnectRule_Error(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	customMessage := "custom error message for direct connection"

	rule := validators.MongoDirectionConnect(hosts).Error(customMessage)
	err := rule.Validate(true)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != customMessage {
		t.Errorf("Error message = %v, want %v", err.Error(), customMessage)
	}
}

func TestMongoDirectionConnectRule_ErrorObject(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	customErr := validation.NewError("custom_code", "custom error object message")

	rule := validators.MongoDirectionConnect(hosts).ErrorObject(customErr)
	err := rule.Validate(true)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != customErr.Error() {
		t.Errorf("Error message = %v, want %v", err.Error(), customErr.Error())
	}

	// Verify it's the custom error code
	if valErr, ok := err.(validation.Error); ok {
		if valErr.Code() != "custom_code" {
			t.Errorf("Error code = %v, want %v", valErr.Code(), "custom_code")
		}
	} else {
		t.Error("Error should be of type validation.Error")
	}
}
