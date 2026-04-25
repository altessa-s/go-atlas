// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ErrDirectionConnectInvalid is returned when DirectConnect is set to true
// while using multiple hosts or SRV connection strings, which is not supported by MongoDB.
var ErrDirectionConnectInvalid = validation.NewError("validation_mongo_direction_connect",
	"cannot be set to true if multiple hosts are specified, or if SRV schema is used")

// MongoDirectionConnect creates a validation rule for MongoDB DirectConnect setting.
// It validates that DirectConnect is not enabled when using multiple hosts or SRV connection strings.
//
// Example:
//
//	validation.Field(&config.DirectConnection, validators.MongoDirectionConnect(config.Hosts))
func MongoDirectionConnect(hosts []string) MongoDirectionConnectRule {
	return MongoDirectionConnectRule{condition: true, err: ErrDirectionConnectInvalid, hosts: hosts}
}

// MongoDirectionConnectRule implements a validation rule for MongoDB DirectConnect configuration.
// It ensures DirectConnect is only used with single-host, non-SRV connection strings.
type MongoDirectionConnectRule struct {
	// err is the validation error to return when the rule fails
	err validation.Error
	// hosts contains the MongoDB host configuration to validate against
	hosts []string
	// condition determines whether this validation rule should be applied
	condition bool
}

// Validate checks if the DirectConnect setting is compatible with the host configuration.
// It returns an error if DirectConnect is true and either multiple hosts are specified
// or any host contains 'srv' indicating an SRV connection string.
func (r MongoDirectionConnectRule) Validate(v any) error {
	if !r.condition {
		return nil
	}

	value, isNil := validation.Indirect(v)
	if isNil {
		return r.err
	}

	if _, ok := value.(bool); ok {
		containsSrvSchema := false
		for _, host := range r.hosts {
			if strings.HasPrefix(strings.ToLower(host), "mongodb+srv://") {
				containsSrvSchema = true
				break
			}
		}

		if len(r.hosts) > 1 || containsSrvSchema {
			return r.err
		}
	}

	return nil
}

// When sets the condition that determines if the validation should be performed.
// This allows conditional validation based on other configuration values.
func (r MongoDirectionConnectRule) When(condition bool) MongoDirectionConnectRule {
	r.condition = condition
	return r
}

// Error sets a custom error message for the validation rule.
// This allows overriding the default error message with context-specific text.
func (r MongoDirectionConnectRule) Error(message string) MongoDirectionConnectRule {
	r.err = r.err.SetMessage(message)
	return r
}

// ErrorObject sets a custom validation.Error for the rule.
// This allows using pre-defined error objects with specific error codes and messages.
func (r MongoDirectionConnectRule) ErrorObject(err validation.Error) MongoDirectionConnectRule {
	r.err = err
	return r
}
