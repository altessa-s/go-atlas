// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, validators.ErrDirectionConnectInvalid.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMongoDirectionConnectRule_When(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	rule := validators.MongoDirectionConnect(hosts).When(false)

	// Should not return error even with invalid configuration because condition is false
	require.NoError(t, rule.Validate(true))

	// With condition true, should return error
	rule = validators.MongoDirectionConnect(hosts).When(true)
	require.Error(t, rule.Validate(true))
}

func TestMongoDirectionConnectRule_Error(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	customMessage := "custom error message for direct connection"

	rule := validators.MongoDirectionConnect(hosts).Error(customMessage)
	err := rule.Validate(true)

	require.Error(t, err)
	require.Equal(t, customMessage, err.Error())
}

func TestMongoDirectionConnectRule_ErrorObject(t *testing.T) {
	hosts := []string{"mongodb://host1:27017", "mongodb://host2:27017"}
	customErr := validation.NewError("custom_code", "custom error object message")

	rule := validators.MongoDirectionConnect(hosts).ErrorObject(customErr)
	err := rule.Validate(true)

	require.Error(t, err)
	require.Equal(t, customErr.Error(), err.Error())

	// Verify it's the custom error code
	valErr, ok := err.(validation.Error)
	require.True(t, ok, "Error should be of type validation.Error")
	require.Equal(t, "custom_code", valErr.Code())
}
