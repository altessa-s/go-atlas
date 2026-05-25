// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func ptrInt(v int) *int { return &v }

// TestGrpc_Validate_RejectsZeroOrNegativeMsgSize is the regression
// guard for the lower bound. A zero / negative MaxRecvMsgSize would
// effectively disable the inbound size limit (the stdlib gRPC server
// treats it as "no limit"), turning every endpoint into an OOM target
// for a single oversized request.
func TestGrpc_Validate_RejectsZeroOrNegativeMsgSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name+"_recv", func(t *testing.T) {
			cfg := Grpc{
				ListenAddress:  "0.0.0.0:7777",
				MaxRecvMsgSize: ptrInt(tc.value),
			}
			err := cfg.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), "maxRecvMsgSize")
		})
		t.Run(tc.name+"_send", func(t *testing.T) {
			cfg := Grpc{
				ListenAddress:  "0.0.0.0:7777",
				MaxSendMsgSize: ptrInt(tc.value),
			}
			err := cfg.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), "maxSendMsgSize")
		})
	}
}

// TestGrpc_Validate_RejectsHugeMsgSize is the regression guard for the
// upper bound. Operators who hand-crafted MaxRecvMsgSize=math.MaxInt32
// would silently disable the practical limit; the cap catches that.
func TestGrpc_Validate_RejectsHugeMsgSize(t *testing.T) {
	t.Parallel()

	cfg := Grpc{
		ListenAddress:  "0.0.0.0:7777",
		MaxRecvMsgSize: ptrInt(MaxGrpcMessageSize + 1),
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "maxRecvMsgSize")
}

// TestGrpc_Validate_AcceptsReasonableMsgSize pins the happy path so a
// future tighter cap would surface here.
func TestGrpc_Validate_AcceptsReasonableMsgSize(t *testing.T) {
	t.Parallel()

	cfg := Grpc{
		ListenAddress:  "0.0.0.0:7777",
		MaxRecvMsgSize: ptrInt(8 * 1024 * 1024), // 8 MiB
		MaxSendMsgSize: ptrInt(8 * 1024 * 1024),
	}

	require.NoError(t, cfg.Validate())
}

// TestGrpc_Validate_NilMsgSize_OK keeps the "unset = stdlib default"
// path working — operators who don't care about the cap shouldn't have
// to set anything.
func TestGrpc_Validate_NilMsgSize_OK(t *testing.T) {
	t.Parallel()

	cfg := Grpc{ListenAddress: "0.0.0.0:7777"}

	require.NoError(t, cfg.Validate())
}
