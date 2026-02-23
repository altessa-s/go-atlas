// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rootcmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
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
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("expected non-empty error")
	}
	if !bytes.Contains(errOut.Bytes(), []byte("Panic recovered")) {
		t.Fatalf("expected panic output on stderr; got: %s", errOut.String())
	}
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
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if out.String() == "" {
		t.Fatalf("expected stdout to be written")
	}
	if errOut.String() != "" {
		t.Fatalf("expected stderr empty, got: %s", errOut.String())
	}
}
