// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// MinNatsMajorVersion is the minimum required NATS server major version.
	MinNatsMajorVersion = 2
	// MinNatsMinorVersion is the minimum required NATS server minor version.
	MinNatsMinorVersion = 11
)

// ErrNatsVersionNotSupported is returned when the NATS server version is less than
// 2.11.0 or JetStream is not enabled.
var ErrNatsVersionNotSupported = errors.New("NATS server version must be 2.11.0 or higher and JetStream must be enabled")

// ValidateNatsVersion checks if the NATS server version meets the minimum requirements (2.11.0+).
// Returns nil if version is valid, ErrNatsVersionNotSupported if version is too old,
// or a wrapped error if version parsing fails.
func ValidateNatsVersion(nc *nats.Conn) error {
	ver := nc.ConnectedServerVersion()
	verParts := strings.Split(ver, ".")
	if len(verParts) < 2 { //nolint:mnd
		return fmt.Errorf("failed to parse NATS version %q: expected at least major.minor", ver)
	}

	major, err := strconv.Atoi(verParts[0])
	if err != nil {
		return coreerrs.WrapOperation(err, fmt.Sprintf("parse NATS major version from %q", ver))
	}
	minor, err := strconv.Atoi(verParts[1])
	if err != nil {
		return coreerrs.WrapOperation(err, fmt.Sprintf("parse NATS minor version from %q", ver))
	}

	if major > MinNatsMajorVersion || (major == MinNatsMajorVersion && minor >= MinNatsMinorVersion) {
		return nil
	}

	return fmt.Errorf("%w: server version %s is less than required %d.%d",
		ErrNatsVersionNotSupported, ver, MinNatsMajorVersion, MinNatsMinorVersion)
}

// ValidateJetStreamEnabled verifies that JetStream is enabled for the given account.
// It logs a consistent message via logger (when non-nil) and returns the underlying error.
func ValidateJetStreamEnabled(ctx context.Context, js jetstream.JetStream, logger *slog.Logger) error {
	_, err := js.AccountInfo(ctx)
	if err == nil {
		return nil
	}

	if errors.Is(err, nats.ErrJetStreamNotEnabled) {
		if logger != nil {
			logger.Error("jetstream is not enabled on the nats server")
		}
		return err
	}

	if logger != nil {
		logger.Error("failed to get JetStream account info", slog.Any("error", err))
	}
	return err
}
