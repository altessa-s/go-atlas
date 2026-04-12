// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rootcmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRun_PanicInSubcommand_ReturnsErrorAndWritesToErr(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	boom := &cobra.Command{
		Use: "boom",
		RunE: func(_ *cobra.Command, _ []string) error {
			panic("boom")
		},
	}

	cfg := &Config{
		Use:         "testtool",
		Short:       "test",
		Subcommands: []*cobra.Command{boom},
		Out:         &out,
		Err:         &errOut,
	}

	err := Run(cfg, []string{"boom"})
	require.Error(t, err)
	require.NotEmpty(t, err.Error())
	require.True(t, bytes.Contains(errOut.Bytes(), []byte("Panic recovered")), "expected panic output on stderr; got: %s", errOut.String())
}

func TestNew_UsesProvidedWriters(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cfg := &Config{
		Use:   "testtool",
		Short: "test",
		Out:   &out,
		Err:   &errOut,
		Subcommands: []*cobra.Command{
			{
				Use: "hello",
				RunE: func(cmd *cobra.Command, _ []string) error {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "hello")
					return nil
				},
			},
		},
	}

	cmd := New(cfg)
	cmd.SetArgs([]string{"hello"})
	require.NoError(t, cmd.Execute())
	require.NotEmpty(t, out.String(), "expected stdout to be written")
	require.Empty(t, errOut.String(), "expected stderr empty")
}
