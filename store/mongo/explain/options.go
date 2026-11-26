// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package explain

//go:generate go tool optgen generate

// Verbosity controls how much work MongoDB performs for an explain command.
type Verbosity string

const (
	// VerbosityQueryPlanner inspects the winning query plan without executing the pipeline.
	VerbosityQueryPlanner Verbosity = "queryPlanner"
	// VerbosityExecutionStats executes the pipeline to collect examined and returned counts.
	VerbosityExecutionStats Verbosity = "executionStats"
	// DefaultVerbosity is the default explain verbosity.
	DefaultVerbosity = VerbosityQueryPlanner
)

type options struct {
	// WithVerbosity sets the MongoDB explain verbosity mode.
	verbosity Verbosity `optgen:"default=DefaultVerbosity,onfail=skip" optcheck:"oneof=[VerbosityQueryPlanner,VerbosityExecutionStats]"`
	// WithInsecureLogRawExplain logs the full MongoDB explain response at Debug level.
	// Raw explain output includes parsedQuery and can expose sensitive filter values.
	insecureLogRawExplain bool
}
